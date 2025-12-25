package gtw

import (
	"net/http"
)

type Authorizer interface {
	Authz(rw http.ResponseWriter, req *http.Request, rec *RecordTrace) bool
}
