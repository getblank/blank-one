package berror

import "errors"

var (
	WrongData    = errors.New("WRONG_DATA")
	ServerError  = errors.New("SERVER_ERROR")
	InvalidQuery = errors.New("INVALID_QUERY")
)
