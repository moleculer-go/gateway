package gateway

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/moleculer-go/moleculer"
	"github.com/moleculer-go/moleculer/payload"
	log "github.com/sirupsen/logrus"
)

type actionHandler struct {
	routePath            string
	alias                string
	action               string
	context              moleculer.Context
	acceptedMethodsCache map[string]bool
}

// aliasPath return the alias path, if one exists for the action.
func (handler *actionHandler) aliasPath() string {
	if handler.alias != "" {
		parts := strings.Split(strings.TrimSpace(handler.alias), " ")
		alias := ""
		if len(parts) == 1 {
			alias = parts[0]
		} else if len(parts) == 2 {
			alias = parts[1]
		} else {
			panic(fmt.Sprint("Invalid alias format! -> ", handler.alias))
		}
		return alias
	}
	return ""
}

// pattern return the path pattern used to map URL in the http.ServeMux
func (handler *actionHandler) pattern() string {
	actionPath := strings.Replace(handler.action, ".", "/", -1)
	fullPath := ""
	aliasPath := handler.aliasPath()
	if aliasPath != "" {
		fullPath = fmt.Sprint(handler.routePath, "/", aliasPath)
	} else {
		fullPath = fmt.Sprint(handler.routePath, "/", actionPath)
	}
	return strings.Replace(fullPath, "//", "/", -1)
}

// invalidHttpMethodError send an error in the reponse about the http method being invalid.
func (handler *actionHandler) invalidHttpMethodError(logger *log.Entry, response http.ResponseWriter, methods map[string]bool) {
	acceptedMethods := []string{}
	for methodName := range methods {
		acceptedMethods = append(acceptedMethods, methodName)
	}
	error := fmt.Errorf("Invalid HTTP Method - accepted methods: %s", acceptedMethods)
	handler.sendReponse(logger, payload.New(error), response)
}

var succesStatusCode = 200
var errorStatusCode = 500
var resultParseErrorStatusCode = 500

// sendReponse send the result payload  back using the ResponseWriter
func (handler *actionHandler) sendReponse(logger *log.Entry, result moleculer.Payload, response http.ResponseWriter) {
	var json []byte
	response.Header().Add("Content-Type", "application/json")
	if result.IsError() {
		response.WriteHeader(errorStatusCode)
		json = jsonSerializer.PayloadToBytes(payload.Empty().Add("error", result.Error().Error()))
	} else {
		response.WriteHeader(succesStatusCode)
		json = jsonSerializer.PayloadToBytes(result)
	}
	logger.Debug("Gateway SendReponse() - action: ", handler.action, " json: ", string(json), " result.IsError(): ", result.IsError())
	response.Write(json)
}

func (handler *actionHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	methods := handler.acceptedMethods()
	logger := handler.context.Logger()
	switch request.Method {
	case http.MethodGet:
		if methods["GET"] {
			handler.sendReponse(logger, <-handler.context.Call(handler.action, paramsFromRequest(request, logger)), response)
		}
	case http.MethodPost:
		if methods["POST"] {
			handler.sendReponse(logger, <-handler.context.Call(handler.action, paramsFromRequest(request, logger)), response)
		}
	case http.MethodPut:
		if methods["PUT"] {
			handler.sendReponse(logger, <-handler.context.Call(handler.action, paramsFromRequest(request, logger)), response)
		}
	case http.MethodDelete:
		if methods["DELETE"] {
			handler.sendReponse(logger, <-handler.context.Call(handler.action, paramsFromRequest(request, logger)), response)
		}
	default:
		handler.invalidHttpMethodError(logger, response, methods)
	}
}

//acceptedMethods return a map of accepted methods for this handler.
func (handler *actionHandler) acceptedMethods() map[string]bool {
	if handler.acceptedMethodsCache != nil {
		return handler.acceptedMethodsCache
	}
	if handler.alias != "" {
		parts := strings.Split(strings.TrimSpace(handler.alias), " ")
		if len(parts) == 2 {
			method := strings.ToUpper(parts[0])
			if validMethod(method) {
				handler.acceptedMethodsCache = map[string]bool{
					method: true,
				}
				return handler.acceptedMethodsCache
			}
		}
	}
	handler.acceptedMethodsCache = map[string]bool{
		"GET":    true,
		"POST":   true,
		"PUT":    true,
		"DELETE": true,
	}
	return handler.acceptedMethodsCache
}
