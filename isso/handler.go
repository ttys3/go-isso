package isso

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"github.com/kr/pretty"
	"wrong.wang/x/go-isso/logger"
	"wrong.wang/x/go-isso/response/json"
	"wrong.wang/x/go-isso/tool/validator"
)

// CreateComment create a new comment
func (isso *ISSO) CreateComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := RequestIDFromContext(r.Context())
		commentWebsite := FindOrigin(r)
		if commentWebsite == "" {
			json.BadRequest(requestID, w, nil, "can not find header origin")
			return
		}
		var comment submittedComment
		err := jsonBind(r.Body, &comment)
		if err != nil {
			json.BadRequest(requestID, w, err, descRequestInvalidParm)
			return
		}
		comment.URI = mux.Vars(r)["uri"]
		comment.RemoteAddr = findClientIP(r)
		if err := validator.Validate(comment); err != nil {
			json.BadRequest(requestID, w, err, fmt.Sprintf("comment validate failed: %s", err.Error()))
			return
		}
		pretty.Println(comment)

		var thread Thread
		thread, err = isso.storage.GetThreadByURI(r.Context(), comment.URI)
		if err != nil {
			if errors.Is(err, ErrStorageNotFound) {
				// no thread realted to this uri
				// so create new thread
				if thread, err = isso.storage.NewThread(r.Context(), comment.URI, comment.Title, commentWebsite); err != nil {
					json.ServerError(requestID, w, err, descStorageUnhandledError)
					return
				}
			} else {
				// can not handled error
				json.ServerError(requestID, w, err, descStorageUnhandledError)
				return
			}
		}

		if isso.config.Moderation.Enable {
			if isso.config.Moderation.ApproveAcquaintance &&
				comment.Email != nil &&
				isso.storage.IsApprovedAuthor(r.Context(), *comment.Email) {
				comment.Mode = 1
			} else {
				comment.Mode = 2
			}
		} else {
			comment.Mode = 1
		}

		c, err := isso.storage.NewComment(r.Context(), comment.Comment, thread.ID, comment.RemoteAddr)
		if err != nil {
			json.ServerError(requestID, w, err, descStorageUnhandledError)
			return
		}

		logger.Debug(fmt.Sprintf("new comment: %# v", pretty.Formatter(c)))

		if encoded, err := isso.tools.securecookie.Encode(fmt.Sprintf("%v", c.ID),
			map[int64][20]byte{c.ID: sha1.Sum([]byte(c.Text))}); err == nil {
			cookie := &http.Cookie{
				Name:   fmt.Sprintf("%v", c.ID),
				Value:  encoded,
				Path:   "/",
				MaxAge: isso.config.MaxAge,
				Secure: true,
			}
			http.SetCookie(w, cookie)
			cookie = &http.Cookie{
				Name:   fmt.Sprintf("isso-%v", c.ID),
				Value:  encoded,
				Path:   "/",
				MaxAge: isso.config.MaxAge,
				Secure: true,
			}
			if v := cookie.String(); v != "" {
				w.Header().Add("X-Set-Cookie", v)
			}
		}

		if c.Mode == 2 {
			json.Accepted(w, c)
		} else {
			json.Created(w, c)
		}
	}
}

// FetchComments fetch all related comments
func (isso *ISSO) FetchComments() http.HandlerFunc {
	type urlParm struct {
		Parent      *int64  `schema:"parent"`
		Limit       int64   `schema:"limit"`
		NestedLimit int64   `schema:"nested_limit"`
		After       float64 `schema:"after"`
		Plain       int64   `schema:"plain"`
	}
	decoder := schema.NewDecoder()
	decoder.IgnoreUnknownKeys(true)

	makeReplies := func(cs []Comment, after float64, limit int64, plain bool) []reply {
		var replies []reply
		var count int64
		if limit <= 0 {
			limit = int64(len(cs) + 1)
		}
		for _, c := range cs {
			if c.Created > after && count < limit {
				count++
				r, _ := c.convert(plain, isso.tools.hash, isso.tools.markdown)
				replies = append(replies, r)
			}
		}
		return replies
	}

	return func(w http.ResponseWriter, r *http.Request) {
		requestID := RequestIDFromContext(r.Context())
		var urlparm urlParm
		err := decoder.Decode(&urlparm, r.URL.Query())
		if err != nil {
			json.BadRequest(requestID, w, err, descRequestInvalidParm)
			return
		}
		var parent int64
		if urlparm.Parent == nil {
			parent = -1
		} else {
			parent = *urlparm.Parent
		}
		var plain bool
		if urlparm.Plain != 0 {
			plain = true
		}

		replyCount, err := isso.storage.CountReply(r.Context(), mux.Vars(r)["uri"], 5, urlparm.After)
		if err != nil {
			json.ServerError(requestID, w, err, descStorageUnhandledError)
			return
		}
		// param `after` may cause the loss of old comment's parent
		if _, ok := replyCount[parent]; !ok {
			replyCount[parent] = 0
		}

		commentsByParent, err := isso.storage.FetchCommentsByURI(r.Context(), mux.Vars(r)["uri"], parent, 5, "id", true)
		if err != nil {
			json.ServerError(requestID, w, err, descStorageUnhandledError)
			return
		}
		rJSON := struct {
			TotalReplies  int64   `json:"total_replies"`
			Replies       []reply `json:"replies"`
			ID            *int64  `json:"id"`
			HiddenReplies int64   `json:"hidden_replies"`
		}{
			ID: urlparm.Parent,
		}

		// null parent, only fetch top-comment
		if parent == -1 {
			// parent == -1 means need all comment's, here TotalReplies means top-leval comments
			rJSON.TotalReplies = replyCount[0]

			rJSON.Replies = makeReplies(commentsByParent[0], urlparm.After, urlparm.Limit, plain)
			rJSON.HiddenReplies = rJSON.TotalReplies - int64(len(rJSON.Replies))
			var zero int64
			emptyarray := make([]reply, 0)
			for i := range rJSON.Replies {
				count, ok := replyCount[rJSON.Replies[i].ID]
				if !ok {
					rJSON.Replies[i].TotalReplies = &zero
					rJSON.Replies[i].Replies = &emptyarray
					rJSON.Replies[i].HiddenReplies = &zero
				} else {
					replies := makeReplies(commentsByParent[rJSON.Replies[i].ID], urlparm.After, urlparm.NestedLimit, plain)
					rJSON.Replies[i].TotalReplies = &count
					rJSON.Replies[i].Replies = &replies
					cc := *rJSON.Replies[i].TotalReplies - int64(len(*rJSON.Replies[i].Replies))
					rJSON.Replies[i].HiddenReplies = &cc
				}
			}

		} else if parent > 0 {
			rJSON.TotalReplies = replyCount[parent]
			rJSON.Replies = makeReplies(commentsByParent[parent], urlparm.After, urlparm.Limit, plain)
			rJSON.HiddenReplies = rJSON.TotalReplies - int64(len(rJSON.Replies))
		} else {
			// parent = 0 not exist
			rJSON.TotalReplies = 0
			rJSON.Replies = []reply{}
			rJSON.HiddenReplies = 0
		}
		json.OK(w, rJSON)
	}
}

// CountComment return every thread's comment amount
func (isso *ISSO) CountComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := RequestIDFromContext(r.Context())
		uris := []string{}
		err := jsonBind(r.Body, &uris)
		if err != nil {
			json.BadRequest(requestID, w, err, descRequestInvalidParm)
			return
		}

		counts := []int64{}
		if len(uris) == 0 {
			json.OK(w, counts)
			return
		}
		countsByURI, err := isso.storage.CountComment(r.Context(), uris)
		if err != nil {
			json.ServerError(requestID, w, err, descStorageUnhandledError)
			return
		}
		for _, i := range countsByURI {
			counts = append(counts, i)
		}
		json.OK(w, counts)
	}
}

// ViewComment return specific comment
func (isso *ISSO) ViewComment() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		requestID := RequestIDFromContext(req.Context())
		id, err := strconv.ParseInt(mux.Vars(req)["id"], 10, 64)
		if err != nil {
			json.BadRequest(requestID, w, err, descRequestInvalidParm)
			return
		}

		var plain bool
		if req.URL.Query().Get("plain") == "0" {
			plain = true
		}

		comment, err := isso.storage.GetComment(req.Context(), id)
		if err != nil {
			if errors.Is(err, ErrStorageNotFound) {
				json.NotFound(requestID, w, err, descStorageNotFound)
				return
			}
			json.ServerError(requestID, w, err, descStorageUnhandledError)
			return
		}

		r, _ := comment.convert(plain, isso.tools.hash, isso.tools.markdown)
		json.OK(w, r)
	}
}

// EditComment edit an existing comment.
// Editing a comment is only possible for a short period of time after it was created and only if the requestor has a valid cookie for it.
// Editing a comment will set a new edit cookie in the response.
func (isso *ISSO) EditComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

	}
}

// VoteComment used to like or dislike comment
func (isso *ISSO) VoteComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

	}
}

// DeleteComment delete a comment
func (isso *ISSO) DeleteComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

	}
}
