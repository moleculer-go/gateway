package websocket_test

import (
	"fmt"

	"github.com/moleculer-go/moleculer"
	"github.com/moleculer-go/moleculer/broker"
	"github.com/moleculer-go/moleculer/payload"
	"github.com/moleculer-go/moleculer/serializer"
	"github.com/moleculer-go/moleculer/transit/memory"
	. "github.com/onsi/ginkgo"
	log "github.com/sirupsen/logrus"
)

var jsonSerializer = serializer.CreateJSONSerializer(log.WithField("gateway", "json-serializer"))
var logLevel = "info"

func createGatewayBroker(mem *memory.SharedMemory) broker.ServiceBroker {
	broker := broker.New(&moleculer.Config{
		DiscoverNodeID: func() string { return "node_gatewayBroker" },
		LogLevel:       logLevel,
		TransporterFactory: func() interface{} {
			transport := memory.Create(log.WithField("transport", "memory"), mem)
			return &transport
		},
	})
	return (*broker)
}

func createEchoBroker(mem *memory.SharedMemory, prefix string) broker.ServiceBroker {
	broker := broker.New(&moleculer.Config{
		DiscoverNodeID: func() string { return "node_tempBroker" },
		LogLevel:       logLevel,
		TransporterFactory: func() interface{} {
			transport := memory.Create(log.WithField("transport", "memory"), mem)
			return &transport
		},
	})
	broker.Publish(moleculer.ServiceSchema{
		Name: "echo",
		Started: func(ctx moleculer.BrokerContext, svc moleculer.ServiceSchema) {
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
		// mem := &memory.SharedMemory{}
		// echoBkr := createEchoBroker(mem, "uhuuu")
		// gatewayBkr := createGatewayBroker(mem)
		// gatewaySvc := gateway.HttpService{Settings: map[string]interface{}{
		// 	"port": "3752",
		// }}
		// gatewayBkr.Publish(gatewaySvc)
		// echoBkr.Start()
		// gatewayBkr.Start()

		// socket := gowebsocket.New("ws://localhost:3752/ws/")
		// connected := make(chan bool)
		// socket.OnConnected = func(socket gowebsocket.Socket) {
		// 	fmt.Println("** Connected to server ** ")
		// 	connected <- true
		// }
		// socket.Connect()
		// Expect(socket).ShouldNot(BeNil())

		// msgReceived := make(chan string)
		// socket.OnTextMessage = func(message string, socket gowebsocket.Socket) {
		// 	fmt.Println("Recieved message " + message)
		// 	msgReceived <- message
		// }
		// Expect(<-connected).Should(Equal(true))

		// fmt.Println("connected, will send msg")

		// //subscribe to echo.gone
		// socket.SendText(`{"event":"subscribe", "payload":{"name":"N", "value":"V", "topic":"echo.gone"}}`)

		// //Expect(err).Should(BeNil())

		// fmt.Println("Message sent :)")

		// gatewayBkr.Call("echo.go", map[string]interface{}{
		// 	"content": "music...",
		// })

		// msg := <-msgReceived
		// // var msg string
		// // err = websocket.Message.Receive(conn, &msg)
		// fmt.Println("msg -> ", msg)
		// bts := []byte(msg)
		// pl := jsonSerializer.BytesToPayload(&bts)

		// Expect(pl.Get("result").String()).Should(Equal("music... uhuuu ..."))

		// echoBkr.Stop()
		// gatewayBkr.Stop()
	})

})
