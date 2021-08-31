package berror

import "errors"

var (
	DbNotConnected     = errors.New("NO_DB_CONNECTION")
	DbNotFound         = errors.New("not found")
	DbNotOwn           = errors.New("NOT_AUTHORIZED")
	DbCreateNotAllowed = errors.New("CREATE_NOT_ALLOWED")
	DbReadNotAllowed   = errors.New("READ_NOT_ALLOWED")
	DbUpdateNotAllowed = errors.New("UPDATE_NOT_ALLOWED")
	DbDeleteNotAllowed = errors.New("DELETE_NOT_ALLOWED")
	DbCantExec         = errors.New("CAN_NOT_EXEC")
	DbIndexExists      = errors.New("INDEX_EXISTS")
	DbObjDeleted       = errors.New("OBJECT_DELETED")
	DbNoIndexName      = errors.New("NO_INDEX_NAME")
	DbIndexNotFound    = errors.New("NO_INDEX_NAME")
)
