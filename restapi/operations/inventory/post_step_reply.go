// Code generated by go-swagger; DO NOT EDIT.

package inventory

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the generate command

import (
	"net/http"

	"github.com/go-openapi/runtime/middleware"
)

// PostStepReplyHandlerFunc turns a function with the right signature into a post step reply handler
type PostStepReplyHandlerFunc func(PostStepReplyParams) middleware.Responder

// Handle executing the request and returning a response
func (fn PostStepReplyHandlerFunc) Handle(params PostStepReplyParams) middleware.Responder {
	return fn(params)
}

// PostStepReplyHandler interface for that can handle valid post step reply params
type PostStepReplyHandler interface {
	Handle(PostStepReplyParams) middleware.Responder
}

// NewPostStepReply creates a new http.Handler for the post step reply operation
func NewPostStepReply(ctx *middleware.Context, handler PostStepReplyHandler) *PostStepReply {
	return &PostStepReply{Context: ctx, Handler: handler}
}

/*PostStepReply swagger:route POST /nodes/{node_id}/next-steps/reply inventory postStepReply

Post the result of the required operations from the server

*/
type PostStepReply struct {
	Context *middleware.Context
	Handler PostStepReplyHandler
}

func (o *PostStepReply) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	route, rCtx, _ := o.Context.RouteInfo(r)
	if rCtx != nil {
		r = rCtx
	}
	var Params = NewPostStepReplyParams()

	if err := o.Context.BindValidRequest(r, route, &Params); err != nil { // bind params
		o.Context.Respond(rw, r, route.Produces, route, err)
		return
	}

	res := o.Handler.Handle(Params) // actually handle the request

	o.Context.Respond(rw, r, route.Produces, route, res)

}