package gateway

import (
	"io"
	"net/http"
	"sync"

	"github.com/moleculer-go/moleculer/payload"
	"github.com/moleculer-go/moleculer/util"

	"github.com/moleculer-go/moleculer"
	"golang.org/x/net/websocket"
)

type topicEntry struct {
	topic   string
	context moleculer.BrokerContext
	started bool
	clients []*WebSocketClient
}

// validate check if params should be added in the event stream
func (te *topicEntry) validate(params moleculer.Payload, client *WebSocketClient) bool {
	pvalue := params.Get(client.name)
	if pvalue.Exists() && pvalue.String() == client.value {
		te.context.Logger().Debug("socker.io event validate() - record is valid !")
		return true
	}
	te.context.Logger().Debug("socker.io event validate() - record is invalid :(")
	return false
}

// websocketFunnelService create a service schema for events funnel handler.
// which will listen for the events -> topic and call the handler.
func websocketFunnelService(topic string, handler moleculer.EventHandler) moleculer.Service {
	return moleculer.Service{
		Name: "websocket-events_funnel-" + topic,
		Events: []moleculer.Event{
			{
				Name:    topic,
				Handler: handler,
			},
		},
	}
}

// start create a moleculer service to listen for the events on the topic.
// when event occours deliver to the client.
func (te *topicEntry) start() {
	if te.started {
		te.context.Logger().Debug("addClient() topic already started! topic name: ", te.topic)
		return
	}
	te.started = true
	te.context.Logger().Debug("start() for topic: ", te.topic)

	te.context.AddService(websocketFunnelService(te.topic, func(context moleculer.Context, params moleculer.Payload) {
		context.Logger().Debug("event handler for topic: ", te.topic, " received params: ", params)
		for _, client := range te.clients {
			if te.validate(params, client) {
				target := client.value + "." + te.topic
				client.pub(target, params)
			}
		}
	}))
}

// addClient start the delivery of moleculer events to socket events.
func (te *topicEntry) addClient(client *WebSocketClient) {
	te.clients = append(te.clients, client)
	//when client close.. remove from the list of clients
	//inside hte topic entry
	client.onClose = func() {
		list := make([]*WebSocketClient, len(te.clients)-1)
		for _, item := range te.clients {
			if item.id != client.id {
				list = append(list, item)
			}
		}
		te.clients = list
	}
	te.start()
}

type WebSocketClient struct {
	server      *WebSocketPubSub
	conn        *websocket.Conn
	id          string
	outChan     chan Message
	inChan      chan string
	receiveDone chan bool
	sendDone    chan bool

	name           string
	value          string
	onClose        func()
	receiveMessage func(conn *websocket.Conn) (Message, error)
	sendMessage    func(conn *websocket.Conn, msg Message) error
}

func newWebSocketClient(server *WebSocketPubSub, conn *websocket.Conn, id string) *WebSocketClient {
	return &WebSocketClient{
		server:      server,
		conn:        conn,
		id:          id,
		outChan:     make(chan Message),
		inChan:      make(chan string),
		receiveDone: make(chan bool, 1),
		sendDone:    make(chan bool, 1),
		onClose:     func() {},
		receiveMessage: func(conn *websocket.Conn) (Message, error) {
			var msg Message
			err := websocket.JSON.Receive(conn, &msg)
			return msg, err
		},
		sendMessage: func(conn *websocket.Conn, msg Message) error {
			return websocket.JSON.Send(conn, msg)
		},
	}
}

// pub publishes a message to the client.
func (wc *WebSocketClient) pub(topic string, payload moleculer.Payload) {
	msg := Message{topic, payload.RawMap()}
	go func(outChan chan Message) { outChan <- msg }(wc.outChan)
}

type Message struct {
	event   string                 `json:"event"`
	payload map[string]interface{} `json:"payload"`
}

// start stats both receive and send pumps.
// it means that the client will start receiving and sending msgs.
func (wc *WebSocketClient) start() {
	go wc.receive()
	go wc.send()
}

// stop both receive and send pumps.
//It means the client will longer send or receive any messages.
func (wc *WebSocketClient) stop() {
	wc.sendDone <- true
	wc.receiveDone <- true
}

//send loops while there is not a doneChan signal
//and on each look check if there is output in the outChan
// if there is sends a message down the websocket to the client.
func (wc *WebSocketClient) send() {
	for {
		select {
		case <-wc.sendDone:
			wc.onClose()
			return
		// send data to client
		case msg := <-wc.outChan:
			err := wc.sendMessage(wc.conn, msg)
			if err == io.EOF {
				wc.stop()
				return
			} else if err != nil {
				wc.server.context.Logger().Trace("Error sending msg - error: ", err)
			}
		}
	}
}

// receive loops while there is not a doneChan signal
// and one each loop check if there is a JSON message on the
// websocket. where there is delivers to the message func.
func (wc *WebSocketClient) receive() {
	for {
		select {
		case <-wc.receiveDone:
			wc.onClose()
			return
		// read data from websocket connection
		default:
			msg, err := wc.receiveMessage(wc.conn)
			if err == io.EOF {
				wc.stop()
				return
			} else if err != nil {
				wc.server.context.Logger().Trace("Error receiving json msg - error: ", err)
			} else {
				go wc.server.pub(wc, msg.event, payload.New(msg.payload))
			}
		}

	}
}

type WebSocketPubSub struct {
	clients       *sync.Map
	subscriptions *sync.Map
	clientTopics  *sync.Map
	ws            *websocket.Server
	context       moleculer.BrokerContext
}

func (ps *WebSocketPubSub) Handler() http.Handler {
	return ps.ws
}

func (ps *WebSocketPubSub) init() {
	ps.ws.Handler = func(conn *websocket.Conn) {
		id := util.RandomString(12)
		client := newWebSocketClient(ps, conn, id)
		ps.clients.Store(id, client)
		go client.start()
	}
	ps.sub("subscribe", ps.onSubscribe)
}

// pub sends/publish a messages to the listener.
func (ps *WebSocketPubSub) pub(client *WebSocketClient, topic string, params moleculer.Payload) int {
	temp, exist := ps.subscriptions.Load(topic)
	if !exist {
		return 0
	}
	affected := 0
	for _, handler := range temp.([]subHandler) {
		go handler(client, params)
		affected++
	}
	return affected
}

type subHandler func(client *WebSocketClient, params moleculer.Payload)

// sub subscribe to a topic and get notified using the handler when an event is sent to the same topic.
func (ps *WebSocketPubSub) sub(topic string, handler subHandler) {
	temp, exist := ps.subscriptions.Load(topic)
	if !exist {
		ps.subscriptions.Store(topic, []subHandler{handler})
		return
	}
	ps.subscriptions.Store(topic, append(temp.([]subHandler), handler))
}

// getOrCreateTopic if the topic exists return the topicEntry,
// is not creates one, store in it and return it.
func (ps *WebSocketPubSub) getOrCreateTopic(topic string) *topicEntry {
	temp, exists := ps.clientTopics.Load(topic)
	if exists {
		ps.context.Logger().Debug("getTopicEntry topicEntry found for topic: ", topic)
		te := temp.(*topicEntry)
		return te
	}
	te := topicEntry{context: ps.context, topic: topic}
	ps.clientTopics.Store(topic, &te)
	temp, _ = ps.clientTopics.Load(topic)
	return temp.(*topicEntry)
}

// onSubscribe is called when a client subscribes to a topic
// to receive moleculer events on a websocket connection.
func (ps *WebSocketPubSub) onSubscribe(client *WebSocketClient, params moleculer.Payload) {
	logger := ps.context.Logger()
	logger.Debug("onSubscribe params: ", params)

	topic := params.Get("topic").String()
	client.name = params.Get("name").String()
	client.value = params.Get("value").String()

	te := ps.getOrCreateTopic(topic)
	te.addClient(client)

	target := client.value + "." + te.topic + ".setup"
	logger.Debug("onSubscribe Delivery Started! -> target: ", target)
}

func NewWebSocketPubSub(context moleculer.BrokerContext) *WebSocketPubSub {
	ps := &WebSocketPubSub{
		clients:       &sync.Map{},
		subscriptions: &sync.Map{},
		clientTopics:  &sync.Map{},
		ws:            &websocket.Server{},
		context:       context,
	}
	ps.init()
	return ps
}
