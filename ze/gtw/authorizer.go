package gtw

import (
	"net/http"
)

type Authorizer interface {
	Authz(gw IGateway, rw http.ResponseWriter, rr *http.Request, rt *RecordTrace) bool
}
