package database

var (
	presetSQLITE3 map[string]string = map[string]string{
		"create": `
		CREATE TABLE IF NOT EXISTS comments (
			tid REFERENCES threads(id), 
			id INTEGER PRIMARY KEY, 
			parent INTEGER,
			created FLOAT NOT NULL,
			modified FLOAT,
			mode INTEGER,
			remote_addr VARCHAR,
			text VARCHAR,
			author VARCHAR,
			email VARCHAR,
			website VARCHAR,
			likes INTEGER DEFAULT 0,
			dislikes INTEGER DEFAULT 0,
			voters BLOB NOT NULL,
			notification INTEGER DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS preferences (
			key VARCHAR PRIMARY KEY, 
			value VARCHAR
		);
		CREATE TABLE IF NOT EXISTS threads (
			id INTEGER PRIMARY KEY,
			uri VARCHAR(256) UNIQUE,
			title VARCHAR(256)
		);
		CREATE TRIGGER IF NOT EXISTS remove_stale_threads
    	AFTER DELETE ON comments
    	BEGIN
    		DELETE FROM threads WHERE id NOT IN (SELECT tid FROM comments);
    	END;
		`,
		"migrate_add_notification": `ALTER TABLE comments ADD COLUMN notification INTEGER DEFAULT 0;`,
		
		"preference_get": `SELECT value FROM preferences WHERE key=$1;`,
		"preference_set": `INSERT INTO preferences (key, value) VALUES ($1, $2);`,

		"thread_get_by_uri": `SELECT * FROM threads WHERE uri=$1;`,
		"thread_get_by_id": `SELECT * FROM threads WHERE id=$1;`,
		"thread_new": `INSERT INTO threads (uri, title) VALUES ($1, $2);`,

		"comment_new": `INSERT INTO comments (
        	tid, parent, created, modified, mode, remote_addr,
			text, author, email, website, voters, notification
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);`,
		"comment_get_by_id": `SELECT * FROM comments WHERE id=$1`,
		"comment_is_previously_approved_author": `SELECT CASE WHEN EXISTS(
			SELECT * FROM comments WHERE email=$1 AND mode=1 AND created > strftime("%s", DATETIME("now", "-6 month"))
		) THEN 1 ELSE 0 END;
		`,
	}
)


var presetSQL map[string]map[string]string = map[string]map[string]string{
	"sqlite3": presetSQLITE3,
}