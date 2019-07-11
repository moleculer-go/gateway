package gateway

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"

	"github.com/moleculer-go/moleculer"
	"github.com/moleculer-go/moleculer/context"
	"github.com/moleculer-go/moleculer/test"

	"github.com/tidwall/gjson"

	"github.com/moleculer-go/moleculer/payload"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
)

type mockReponseWriter struct {
	buffer     []byte
	statusCode int
	header     http.Header
}

func (w *mockReponseWriter) Header() http.Header {
	return w.header
}

func (w *mockReponseWriter) Write(bytes []byte) (int, error) {
	w.buffer = append(w.buffer, bytes...)
	return len(bytes), nil
}

func (w *mockReponseWriter) String() string {
	return string(w.buffer)
}

func (w *mockReponseWriter) StatusCode() int {
	return w.statusCode
}

func (w *mockReponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

var _ = Describe("API Gateway", func() {

	Describe("filterActions", func() {
		services := []map[string]interface{}{
			{
				"name": "user",
				"actions": map[string]map[string]interface{}{
					"list": {
						"name":    "user.list",
						"rawName": "list",
					},
					"update": {
						"name":    "user.update",
						"rawName": "update",
					},
				},
			},
			{
				"name": "auth",
				"actions": map[string]map[string]interface{}{
					"login": {
						"name":    "auth.login",
						"rawName": "login",
					},
					"logout": {
						"name":    "auth.logout",
						"rawName": "logout",
					},
				},
			},
		}
		bkrContext := context.BrokerContext(test.DelegatesWithIdAndConfig(
			"nodeID",
			moleculer.Config{},
		))
		ctx := bkrContext.(moleculer.Context)
		It("should create a list of action handles", func() {
			settings := map[string]interface{}{
				"routes": []map[string]interface{}{
					{
						"path": "/",
					},
				},
			}
			actionHandlers := filterActions(ctx, settings, services)
			Expect(len(actionHandlers)).Should(Equal(4))
			sort.Sort(handlerSorter{actionHandlers})
			Expect(actionHandlers[0].pattern()).Should(Equal("/auth/login"))
			Expect(actionHandlers[1].pattern()).Should(Equal("/auth/logout"))
			Expect(actionHandlers[2].pattern()).Should(Equal("/user/list"))
			Expect(actionHandlers[3].pattern()).Should(Equal("/user/update"))
		})

		It("should filter actions using whitelist settings", func() {
			settings := map[string]interface{}{
				"routes": []map[string]interface{}{
					{
						"path":      "/",
						"whitelist": []string{"*.update"},
					},
				},
			}
			actionHandlers := filterActions(ctx, settings, services)
			Expect(len(actionHandlers)).Should(Equal(1))
			Expect(actionHandlers[0].pattern()).Should(Equal("/user/update"))

			settings = map[string]interface{}{
				"routes": []map[string]interface{}{
					{
						"path":      "/",
						"whitelist": []string{"user.*"},
					},
				},
			}
			actionHandlers = filterActions(ctx, settings, services)
			Expect(len(actionHandlers)).Should(Equal(2))
			patterns := actionHandlers[0].pattern() + " " + actionHandlers[1].pattern()
			Expect(patterns).Should(ContainSubstring("/user/list"))
			Expect(patterns).Should(ContainSubstring("/user/update"))
		})

		It("should handle multiple routes", func() {
			settings := map[string]interface{}{
				"routes": []map[string]interface{}{
					{
						"path":      "/",
						"whitelist": []string{"user.*"},
					},
					{
						"path":      "/admin",
						"whitelist": []string{"auth.*"},
					},
				},
			}
			actionHandlers := filterActions(ctx, settings, services)
			Expect(len(actionHandlers)).Should(Equal(4))
			sort.Sort(handlerSorter{actionHandlers})
			Expect(actionHandlers[0].pattern()).Should(Equal("/admin/auth/login"))
			Expect(actionHandlers[1].pattern()).Should(Equal("/admin/auth/logout"))

			Expect(actionHandlers[2].pattern()).Should(Equal("/user/list"))
			Expect(actionHandlers[3].pattern()).Should(Equal("/user/update"))

			settings = map[string]interface{}{
				"routes": []map[string]interface{}{
					{
						"path": "/A",
					},
					{
						"path": "/B",
					},
					{
						"path": "/C",
					},
				},
			}
			actionHandlers = filterActions(ctx, settings, services)
			Expect(len(actionHandlers)).Should(Equal(12))
			sort.Sort(handlerSorter{actionHandlers})
			Expect(actionHandlers[0].pattern()).Should(Equal("/A/auth/login"))
			Expect(actionHandlers[4].pattern()).Should(Equal("/B/auth/login"))
			Expect(actionHandlers[8].pattern()).Should(Equal("/C/auth/login"))
		})
	})

	Describe("sendReponse", func() {
		It("should convert result into JSON and send in the reponse with success status code", func() {
			result := map[string]interface{}{
				"name":     "John",
				"lastName": "Snow",
				"category": "Bastart",
				"nilvalue": nil,
			}

			response := &mockReponseWriter{header: map[string][]string{}}
			ah := actionHandler{}
			ah.sendReponse(log.WithField("test", ""), payload.New(result), response)
			json := response.String()
			Expect(gjson.Get(json, "category").String()).Should(Equal("Bastart"))
			Expect(gjson.Get(json, "lastName").String()).Should(Equal("Snow"))
			Expect(gjson.Get(json, "name").String()).Should(Equal("John"))

			Expect(response.statusCode).Should(Equal(succesStatusCode))
			Expect(response.Header().Get("Content-Type")).Should(Equal("application/json"))
		})

		It("should convert error result into JSON and send in the reponse with error status code", func() {
			response := &mockReponseWriter{header: map[string][]string{}}
			ah := actionHandler{}
			ah.sendReponse(log.WithField("test", ""), payload.New(errors.New("Some error...")), response)
			json := response.String()
			Expect(gjson.Get(json, "error").String()).Should(Equal("Some error..."))
			Expect(response.statusCode).Should(Equal(errorStatusCode))
			Expect(response.Header().Get("Content-Type")).Should(Equal("application/json"))
		})
	})

	Describe("paramsFromRequest", func() {

		It("should get params from the URL", func() {
			parsedUrl, _ := url.Parse("http://local/path?force=false&name=John&actions=first&actions=second")
			request := &http.Request{
				URL: parsedUrl,
			}

			payload := paramsFromRequest(request, log.WithField("unit", "test"))
			Expect(payload.Get("force").Exists()).Should(BeTrue())
			Expect(payload.Get("force").Bool()).Should(BeFalse())

			Expect(payload.Get("name").Exists()).Should(BeTrue())
			Expect(payload.Get("name").String()).Should(Equal("John"))

			Expect(payload.Get("actions").Exists()).Should(BeTrue())
			Expect(payload.Get("actions").StringArray()).Should(Equal([]string{"first", "second"}))

		})

		It("should get params from the body and URL", func() {
			bodyIo := strings.NewReader(`name=Janet&age=47`)
			request := httptest.NewRequest("POST", "http://local/path?forced=maybe", bodyIo)
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			payload := paramsFromRequest(request, log.WithField("unit", "test"))

			Expect(payload.Get("forced").Exists()).Should(BeTrue())
			Expect(payload.Get("forced").String()).Should(Equal("maybe"))

			Expect(payload.Get("age").Exists()).Should(BeTrue())
			Expect(payload.Get("age").Int()).Should(Equal(47))

			Expect(payload.Get("name").Exists()).Should(BeTrue())
			Expect(payload.Get("name").String()).Should(Equal("Janet"))
		})

	})

	It("acceptedMethods should return accept methodscoming from the alias", func() {
		handler := actionHandler{alias: "GET users"}
		Expect(handler.acceptedMethods()).Should(BeEquivalentTo(map[string]bool{
			"GET": true,
		}))

		handler = actionHandler{alias: "POST users"}
		Expect(handler.acceptedMethods()).Should(BeEquivalentTo(map[string]bool{
			"POST": true,
		}))

		handler = actionHandler{alias: "PUT some/path/big"}
		Expect(handler.acceptedMethods()).Should(BeEquivalentTo(map[string]bool{
			"PUT": true,
		}))

		handler = actionHandler{alias: "DELETE two/paths"}
		Expect(handler.acceptedMethods()).Should(BeEquivalentTo(map[string]bool{
			"DELETE": true,
		}))

		handler = actionHandler{alias: "two/paths"}
		Expect(handler.acceptedMethods()).Should(BeEquivalentTo(map[string]bool{
			"GET":    true,
			"POST":   true,
			"PUT":    true,
			"DELETE": true,
		}))
	})

	It("invertStringMap should swap map keys/values", func() {
		aliases := map[string]string{
			"GET users":  "users.list",
			"POST login": "auth.login",
		}
		inverted := invertStringMap(aliases)
		Expect(inverted).Should(BeEquivalentTo(map[string]string{
			"users.list": "GET users",
			"auth.login": "POST login",
		}))
	})

	Describe("createActionHandlers", func() {

		It("should only create actions with alias", func() {
			route := map[string]interface{}{
				"path":          "/admin",
				"mappingPolicy": "restrict",
				"aliases": map[string]string{
					"GET users": "user.list",
					"login":     "auth.login",
				},
			}

			actionHandlers := createActionHandlers(
				route,
				[]string{"user.list", "user.remove", "auth.login"},
			)
			Expect(len(actionHandlers)).Should(Equal(2))

			Expect(actionHandlers[0].action).Should(Equal("user.list"))
			Expect(actionHandlers[0].pattern()).Should(Equal("/admin/users"))
			Expect(actionHandlers[0].alias).Should(Equal("GET users"))

			Expect(actionHandlers[1].action).Should(Equal("auth.login"))
			Expect(actionHandlers[1].pattern()).Should(Equal("/admin/login"))
			Expect(actionHandlers[1].alias).Should(Equal("login"))
		})

		It("should create action handler with route path and action name", func() {
			route := map[string]interface{}{
				"path": "/api",
			}

			actionHandlers := createActionHandlers(route, []string{"user.list", "auth.login"})
			Expect(len(actionHandlers)).Should(Equal(2))

			Expect(actionHandlers[0].action).Should(Equal("user.list"))
			Expect(actionHandlers[0].pattern()).Should(Equal("/api/user/list"))

			Expect(actionHandlers[1].action).Should(Equal("auth.login"))
			Expect(actionHandlers[1].pattern()).Should(Equal("/api/auth/login"))

			route = map[string]interface{}{
				"path": "/somePrefix/",
			}
			actionHandlers = createActionHandlers(route, []string{"profile.create", "image.upload", "msg.send"})
			Expect(len(actionHandlers)).Should(Equal(3))

			Expect(actionHandlers[0].action).Should(Equal("profile.create"))
			Expect(actionHandlers[0].pattern()).Should(Equal("/somePrefix/profile/create"))
			Expect(actionHandlers[0].action).Should(Equal("profile.create"))
		})
	})

	Describe("shouldInclude", func() {
		var actions = []string{
			"user.list",
			"user.get",
			"user.create",
			"user.remove",
			"user.update",

			"profile.list",
			"profile.get",
			"profile.create",
			"profile.remove",
			"profile.update",

			"auth.login",
			"auth.logout",

			"math.add",
			"math.subtract",
		}
		It("must return true for **", func() {
			for _, action := range actions {
				Expect(shouldInclude([]string{"**"}, action)).Should(BeTrue())
				Expect(shouldInclude([]string{"*.*"}, action)).Should(BeTrue())
			}
		})

		It("must handle action wildcards service.*", func() {
			Expect(shouldInclude([]string{"user.*"}, "user.list")).Should(BeTrue())
			Expect(shouldInclude([]string{"profile.*"}, "user.list")).Should(BeFalse())

			Expect(shouldInclude([]string{"user.*"}, "profile.list")).Should(BeFalse())
			Expect(shouldInclude([]string{"profile.*"}, "profile.list")).Should(BeTrue())
		})

		It("must handle action wildcards *.list", func() {
			Expect(shouldInclude([]string{"*.list"}, "user.list")).Should(BeTrue())
			Expect(shouldInclude([]string{"*.list"}, "profile.list")).Should(BeTrue())
			Expect(shouldInclude([]string{"*.create"}, "user.list")).Should(BeFalse())

			Expect(shouldInclude([]string{"*.create"}, "user.create")).Should(BeTrue())
			Expect(shouldInclude([]string{"*.create"}, "profile.create")).Should(BeTrue())
			Expect(shouldInclude([]string{"*.create"}, "auth.login")).Should(BeFalse())
		})

		It("must handle regular expressions", func() {
			Expect(shouldInclude([]string{".*\\.list"}, "user.list")).Should(BeTrue())
			Expect(shouldInclude([]string{".*\\.list"}, "profile.list")).Should(BeTrue())
			Expect(shouldInclude([]string{".*\\.list"}, "v1.auth.list")).Should(BeTrue())
		})
	})

})

type handlerSorter struct {
	actionHandlers []*actionHandler
}

func (h handlerSorter) Len() int {
	return len(h.actionHandlers)
}

func (h handlerSorter) Less(i, j int) bool {
	return (*h.actionHandlers[i]).pattern() < (*h.actionHandlers[j]).pattern()
}

func (h handlerSorter) Swap(i, j int) {
	iv := h.actionHandlers[i]
	jv := h.actionHandlers[j]

	h.actionHandlers[i] = jv
	h.actionHandlers[j] = iv
}
