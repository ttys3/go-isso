package isso

import (
	"github.com/gorilla/securecookie"
	"wrong.wang/x/go-isso/config"
	"wrong.wang/x/go-isso/logger"
)

// ISSO do the main logical staff
type ISSO struct {
	storage Storage
	config  config.Config
	guard   guard
}

type guard struct {
	sc *securecookie.SecureCookie
}

// New a ISSO instance
func New(cfg config.Config, storage Storage) *ISSO {
	var HashKey, BlockKey string
	var err error
	if HashKey, err = storage.GetPreference("hask-key"); err != nil {
		HashKey = string(securecookie.GenerateRandomKey(64))
		err := storage.SetPreference("hask-key", HashKey)
		if err != nil {
			logger.Fatal("set hash-key failed %w", err)
		}
	}
	if BlockKey, err = storage.GetPreference("block-key"); err != nil {
		BlockKey = string(securecookie.GenerateRandomKey(32))
		err := storage.SetPreference("block-key", BlockKey)
		if err != nil {
			logger.Fatal("set block-key failed %w", err)
		}
	}
	BlockKey = string(securecookie.GenerateRandomKey(32))
	HashKey = string(securecookie.GenerateRandomKey(64))
	return &ISSO{
		config: cfg,
		guard: guard{
			sc: securecookie.New([]byte(HashKey), []byte(BlockKey)),
		},
		storage: storage,
	}
}
