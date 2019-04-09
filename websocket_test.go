package gateway

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/moleculer-go/moleculer"
	"github.com/moleculer-go/moleculer/context"
	"github.com/moleculer-go/moleculer/payload"
	"github.com/moleculer-go/moleculer/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type mockWriter struct {
}

func (w mockWriter) Header() http.Header {
	return http.Header{}
}

func (w mockWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (w mockWriter) WriteHeader(statusCode int) {

}

var _ = Describe("API Gateway WebSockets", func() {

	Describe("WebSocketClient", func() {
		delegates := test.DelegatesWithIdAndConfig(
			"WebSocketClient_node",
			moleculer.Config{},
		)
		bkrContext := context.BrokerContext(delegates)
		ps := NewWebSocketPubSub(bkrContext)

		It("pub should send the msg to the outChan", func() {
			conn := websocket.Conn{}
			wc := newWebSocketClient(ps, &conn, "id")
			topic := "sao.joao"
			params := payload.Empty()
			Consistently(wc.outChan).ShouldNot(Receive())
			wc.pub(topic, params)
			Eventually(wc.outChan).Should(Receive())
		})

		It("receive should stop when a signal is sent on doneChan", func() {
			conn := websocket.Conn{}
			wc := newWebSocketClient(ps, &conn, "id")
			wc.receiveMessage = func(conn *websocket.Conn) (moleculer.Payload, error) {
				return payload.Empty(), nil
			}
			wc.closeConn = func(conn *websocket.Conn) {}
			receiveDone := false
			go func() {
				wc.receive()
				receiveDone = true
			}()
			time.Sleep(time.Millisecond)
			Expect(receiveDone).Should(BeFalse())
			wc.receiveDone <- true
			time.Sleep(time.Millisecond)
			Expect(receiveDone).Should(BeTrue())
		})

		It("receive should stop when receive over 5 errors", func() {
			conn := websocket.Conn{}
			wc := newWebSocketClient(ps, &conn, "id")
			wc.receiveMessage = func(conn *websocket.Conn) (moleculer.Payload, error) {
				return payload.Empty(), errors.New("some error...")
			}
			wc.closeConn = func(conn *websocket.Conn) {}
			receiveDone := false
			go func() {
				wc.receive()
				receiveDone = true
			}()
			Expect(receiveDone).Should(BeFalse())
			time.Sleep(time.Millisecond * 10)
			Expect(receiveDone).Should(BeTrue())
		})

		It("receive read a message and pass on to the server.pub->sub", func() {
			conn := websocket.Conn{}
			wc := newWebSocketClient(ps, &conn, "id")

			endofAtlantis := false
			ps.sub("endof.atlantis", func(client *WebSocketClient, params moleculer.Payload) {
				endofAtlantis = true
			})

			receiveMessageCalled := false
			wc.receiveMessage = func(conn *websocket.Conn) (moleculer.Payload, error) {
				receiveMessageCalled = true
				return payload.Empty().Add("topic", "endof.atlantis").Add("payload", map[string]interface{}{}), nil
			}
			wc.closeConn = func(conn *websocket.Conn) {}
			receiveDone := false
			go func() {
				wc.receive()
				receiveDone = true
			}()
			time.Sleep(time.Millisecond)
			Expect(receiveDone).Should(BeFalse())
			Expect(receiveMessageCalled).Should(BeTrue())
			Expect(endofAtlantis).Should(BeTrue())

			wc.receiveDone <- true
			time.Sleep(time.Millisecond)
			Expect(receiveDone).Should(BeTrue())
		})

		It("send should stop when a signal is sent on doneChan", func() {
			conn := websocket.Conn{}
			wc := newWebSocketClient(ps, &conn, "id")
			wc.sendMessage = func(conn *websocket.Conn, msg moleculer.Payload) error {
				return nil
			}
			wc.prepareConnection = func(conn *websocket.Conn) {}
			wc.closeConn = func(conn *websocket.Conn) {}
			sendDone := false
			go func() {
				wc.send()
				sendDone = true
			}()
			time.Sleep(time.Millisecond)
			Expect(sendDone).Should(BeFalse())
			wc.sendDone <- true
			time.Sleep(time.Millisecond)
			Expect(sendDone).Should(BeTrue())
		})

		It("send should stop when receive an io.EOF", func() {
			conn := websocket.Conn{}
			wc := newWebSocketClient(ps, &conn, "id")
			wc.sendMessage = func(conn *websocket.Conn, msg moleculer.Payload) error {
				return io.EOF
			}
			wc.prepareConnection = func(conn *websocket.Conn) {}
			wc.closeConn = func(conn *websocket.Conn) {}
			sendDone := false
			go func() {
				wc.send()
				sendDone = true
			}()
			Expect(sendDone).Should(BeFalse())
			wc.outChan <- payload.Empty()
			time.Sleep(time.Millisecond)
			Expect(sendDone).Should(BeTrue())
		})

		It("send should ignore other errors", func() {
			conn := websocket.Conn{}
			wc := newWebSocketClient(ps, &conn, "id")
			wc.sendMessage = func(conn *websocket.Conn, msg moleculer.Payload) error {
				return errors.New("some error")
			}
			wc.prepareConnection = func(conn *websocket.Conn) {}
			wc.closeConn = func(conn *websocket.Conn) {}
			sendDone := false
			go func() {
				wc.send()
				sendDone = true
			}()
			Expect(sendDone).Should(BeFalse())
			wc.outChan <- payload.Empty()
			time.Sleep(time.Millisecond * 10)
			Expect(sendDone).Should(BeFalse())
			wc.sendDone <- true
			time.Sleep(time.Millisecond)
			Expect(sendDone).Should(BeTrue())
		})

		It("send should send a message :)", func() {
			conn := websocket.Conn{}
			wc := newWebSocketClient(ps, &conn, "id")
			messageSent := false
			wc.sendMessage = func(conn *websocket.Conn, msg moleculer.Payload) error {
				messageSent = true
				return nil
			}
			wc.prepareConnection = func(conn *websocket.Conn) {}
			wc.closeConn = func(conn *websocket.Conn) {}
			sendDone := false
			go func() {
				wc.send()
				sendDone = true
			}()
			Expect(sendDone).Should(BeFalse())
			Expect(messageSent).Should(BeFalse())

			wc.outChan <- payload.Empty()
			time.Sleep(time.Millisecond * 10)
			Expect(sendDone).Should(BeFalse())
			Expect(messageSent).Should(BeTrue())

			wc.sendDone <- true
			time.Sleep(time.Millisecond)
			Expect(sendDone).Should(BeTrue())
		})

	})

	Describe("WebSocketPubSub", func() {

		It("onSubscribe should create a topic entry anda add the client", func() {
			delegates := test.DelegatesWithIdAndConfig(
				"onSubscribe_node",
				moleculer.Config{},
			)
			addServiceCalled := false
			delegates.AddService = func(svcs ...moleculer.Service) {
				addServiceCalled = true
			}
			bkrContext := context.BrokerContext(delegates)
			ps := NewWebSocketPubSub(bkrContext)
			client := &WebSocketClient{}

			topic := "champ.champ"
			ps.onSubscribe(client, payload.New(map[string]interface{}{
				"topic": topic,
				"name":  "deviceToken",
				"value": "123oikjh",
			}))
			Expect(addServiceCalled).Should(BeTrue())
			value, exists := ps.clientTopics.Load(topic)
			Expect(exists).Should(BeTrue())
			Expect(value).ShouldNot(BeNil())
			te := value.(*topicEntry)
			Expect(te.topic).Should(Equal(topic))
			Expect(len(te.clients)).Should(Equal(1))
		})

		It("onSubscribe should reuse a topic entry and add a new client", func() {
			addServiceCalled := 0
			delegates := test.DelegatesWithIdAndConfig(
				"onSubscribe_node",
				moleculer.Config{},
			)
			delegates.AddService = func(svcs ...moleculer.Service) {
				addServiceCalled++
			}
			bkrContext := context.BrokerContext(delegates)
			ps := NewWebSocketPubSub(bkrContext)
			client := &WebSocketClient{}

			topic := "champ.champ"
			ps.onSubscribe(client, payload.New(map[string]interface{}{
				"topic": topic,
				"name":  "deviceToken",
				"value": "123oikjh",
			}))
			Expect(addServiceCalled).Should(Equal(1))
			value, exists := ps.clientTopics.Load(topic)
			Expect(exists).Should(BeTrue())
			Expect(value).ShouldNot(BeNil())
			te := value.(*topicEntry)
			Expect(te.topic).Should(Equal(topic))
			Expect(len(te.clients)).Should(Equal(1))
			Expect(te.clients[0].name).Should(Equal("deviceToken"))
			Expect(te.clients[0].value).Should(Equal("123oikjh"))

			ps.onSubscribe(client, payload.New(map[string]interface{}{
				"topic": topic,
				"name":  "deviceToken",
				"value": "ssssss",
			}))
			Expect(addServiceCalled).Should(Equal(1))
			value, exists = ps.clientTopics.Load(topic)
			Expect(exists).Should(BeTrue())
			Expect(value).ShouldNot(BeNil())
			te = value.(*topicEntry)
			Expect(te.topic).Should(Equal(topic))
			Expect(len(te.clients)).Should(Equal(2))
			Expect(te.clients[0].name).Should(Equal("deviceToken"))
			Expect(te.clients[0].value).Should(Equal("ssssss"))

			ps.onSubscribe(client, payload.New(map[string]interface{}{
				"topic": topic + ".v2",
				"name":  "deviceToken",
				"value": "ssssss",
			}))
			Expect(addServiceCalled).Should(Equal(2))
			value, exists = ps.clientTopics.Load(topic + ".v2")
			Expect(exists).Should(BeTrue())
			Expect(value).ShouldNot(BeNil())
			te = value.(*topicEntry)
			Expect(te.topic).Should(Equal(topic + ".v2"))
			Expect(len(te.clients)).Should(Equal(1))
			Expect(te.clients[0].name).Should(Equal("deviceToken"))
			Expect(te.clients[0].value).Should(Equal("ssssss"))
		})

		It("sub should create a subscription for the topic and handler", func() {
			noop := func(client *WebSocketClient, params moleculer.Payload) {}
			// delegates.AddService = func(svcs ...moleculer.Service) {
			// 	addServiceCalled++
			// }
			delegates := test.DelegatesWithIdAndConfig(
				"sub_node",
				moleculer.Config{},
			)
			bkrContext := context.BrokerContext(delegates)
			ps := NewWebSocketPubSub(bkrContext)
			topic := "happy.ending"
			ps.sub(topic, noop)
			temp, exists := ps.subscriptions.Load(topic)
			Expect(exists).Should(BeTrue())
			handlers := temp.([]subHandler)
			Expect(len(handlers)).Should(Equal(1))

			ps.sub(topic, noop)
			temp, exists = ps.subscriptions.Load(topic)
			Expect(exists).Should(BeTrue())
			handlers = temp.([]subHandler)
			Expect(len(handlers)).Should(Equal(2))

			topic = "sad.begining"
			ps.sub(topic, noop)
			temp, exists = ps.subscriptions.Load(topic)
			Expect(exists).Should(BeTrue())
			handlers = temp.([]subHandler)
			Expect(len(handlers)).Should(Equal(1))

			ps.sub(topic, noop)
			temp, _ = ps.subscriptions.Load(topic)
			handlers = temp.([]subHandler)
			Expect(len(handlers)).Should(Equal(2))
		})

		Describe("pub", func() {
			delegates := test.DelegatesWithIdAndConfig(
				"pub_node",
				moleculer.Config{},
			)
			bkrContext := context.BrokerContext(delegates)
			client := &WebSocketClient{}

			It("should return 0 when no handlers are  found for the topic", func() {
				ps := NewWebSocketPubSub(bkrContext)
				topic := "lord.rings"
				params := payload.Empty()
				Expect(ps.pub(client, topic, params)).Should(Equal(0))
			})

			It("pub should invoke handlers attached o the topic", func() {
				ps := NewWebSocketPubSub(bkrContext)
				topic := "lord.rings"
				subCalled := false
				ps.sub(topic, func(client *WebSocketClient, params moleculer.Payload) {
					subCalled = true
				})
				params := payload.Empty()
				Expect(ps.pub(client, topic, params)).Should(Equal(1))
				time.Sleep(time.Millisecond * 10)
				Expect(subCalled).Should(BeTrue())

				subCalled = false
				sub2Called := false
				ps.sub(topic, func(client *WebSocketClient, params moleculer.Payload) {
					sub2Called = true
				})
				Expect(ps.pub(client, topic, params)).Should(Equal(2))
				time.Sleep(time.Millisecond * 10)
				Expect(subCalled).Should(BeTrue())
				Expect(sub2Called).Should(BeTrue())
			})
		})

	})

})
