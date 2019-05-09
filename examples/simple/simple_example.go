package main

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/moleculer-go/moleculer"

	gateway "github.com/moleculer-go/moleculer-web"
	"github.com/moleculer-go/moleculer/broker"
)

type UserService struct {
}

func (svc *UserService) Name() string {
	return "user"
}

func (svc *UserService) Greet(params moleculer.Payload) string {
	return "Horay! " + params.Get("name").String() + ", you have called an action using the HTTP gateway!"
}

func main() {
	userSvc := &UserService{}
	gatewaySvc := &gateway.HttpService{
		Settings: map[string]interface{}{"port": "9015"},
		Depends:  []string{"user"},
	}

	bkr := broker.New(&moleculer.Config{LogLevel: "error"})
	bkr.Publish(gatewaySvc, userSvc)
	bkr.Start()

	response, _ := http.Get("http://localhost:9015/user/greet?name=John_Snow")
	// $ Horay! John_Snow, you have called an action using the HTTP gateway!
	fmt.Println(bodyContent(response))

	bkr.Stop()
}

// bodyContent return the response body as string
func bodyContent(resp *http.Response) string {
	defer resp.Body.Close()
	bts, _ := ioutil.ReadAll(resp.Body)
	return string(bts)
}
