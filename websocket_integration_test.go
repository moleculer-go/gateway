package gateway_test

import (
	"fmt"
	"time"

	"github.com/moleculer-go/moleculer"
	gateway "github.com/moleculer-go/moleculer-web"
	"github.com/moleculer-go/moleculer/broker"
	"github.com/moleculer-go/moleculer/payload"
	"github.com/moleculer-go/moleculer/serializer"
	"github.com/moleculer-go/moleculer/transit/memory"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sacOO7/gowebsocket"
	log "github.com/sirupsen/logrus"
)

var jsonSerializer = serializer.CreateJSONSerializer(log.WithField("gateway", "json-serializer"))

func createEchoBroker(mem *memory.SharedMemory, prefix string) broker.ServiceBroker {
	broker := broker.New(&moleculer.Config{
		DiscoverNodeID: func() string { return "node_tempBroker" },
		LogLevel:       logLevel,
		TransporterFactory: func() interface{} {
			transport := memory.Create(log.WithField("transport", "memory"), mem)
			return &transport
		},
	})
	broker.AddService(moleculer.Service{
		Name: "echo",
		Started: func(ctx moleculer.BrokerContext, svc moleculer.Service) {
			fmt.Println("echo service started -> prefix: ", prefix)
		},
		Actions: []moleculer.Action{
			{
				Name: "go",
				Handler: func(ctx moleculer.Context, params moleculer.Payload) interface{} {
					ctx.Logger().Info("I'm echo and I'm echoing the content: ", params.Get("content").String())
					result := params.Get("content").String() + " " + prefix + " ..."
					ctx.Broadcast("echo.gone", payload.Empty().Add("result", result))
					ctx.Logger().Info("Broadcast() sent... ")

					return result
				},
			},
		},
		Events: []moleculer.Event{
			{
				Name: "gone",
				Handler: func(context moleculer.Context, params moleculer.Payload) {
					context.Logger().Info("echo.gone --> ", params.Value())
				},
			},
		},
	})
	return (*broker)
}

var _ = Describe("WebSocket Integration Tests", func() {

	XIt("should listen to a echo.gone event", func() {
		mem := &memory.SharedMemory{}
		echoBkr := createEchoBroker(mem, "uhuuu")
		gatewayBkr := createGatewayBroker(mem)
		gatewaySvc := gateway.Service(map[string]interface{}{
			"port": "3752",
		})
		gatewayBkr.AddService(gatewaySvc)
		echoBkr.Start()
		gatewayBkr.Start()
		time.Sleep(300 * time.Millisecond)

		socket := gowebsocket.New("ws://localhost:3752/ws/")
		connected := make(chan bool)
		socket.OnConnected = func(socket gowebsocket.Socket) {
			fmt.Println("** Connected to server ** ")
			connected <- true
		}
		socket.Connect()
		Expect(socket).ShouldNot(BeNil())

		msgReceived := make(chan string)
		socket.OnTextMessage = func(message string, socket gowebsocket.Socket) {
			fmt.Println("Recieved message " + message)
			msgReceived <- message
		}
		Expect(<-connected).Should(Equal(true))

		fmt.Println("connected, will send msg")

		//subscribe to echo.gone
		socket.SendText(`{"event":"subscribe", "payload":{"name":"N", "value":"V", "topic":"echo.gone"}}`)

		time.Sleep(10 * time.Millisecond)
		//Expect(err).Should(BeNil())

		fmt.Println("Message sent :)")

		gatewayBkr.Call("echo.go", map[string]interface{}{
			"content": "music...",
		})
		time.Sleep(300 * time.Millisecond)

		msg := <-msgReceived
		// var msg string
		// err = websocket.Message.Receive(conn, &msg)
		fmt.Println("msg -> ", msg)
		bts := []byte(msg)
		pl := jsonSerializer.BytesToPayload(&bts)

		Expect(pl.Get("result").String()).Should(Equal("music... uhuuu ..."))

		echoBkr.Stop()
		gatewayBkr.Stop()
	})

})
