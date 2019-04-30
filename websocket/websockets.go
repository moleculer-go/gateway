package websocket

import (
	"net/http"
	"sync"
	"time"

	"github.com/moleculer-go/moleculer/payload"
	"github.com/moleculer-go/moleculer/serializer"
	"github.com/moleculer-go/moleculer/service"
	"github.com/moleculer-go/moleculer/util"
	log "github.com/sirupsen/logrus"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/moleculer-go/moleculer"
)

var jsonSerializer = serializer.CreateJSONSerializer(log.WithField("gateway", "json-serializer"))

type WebSocketClient struct {
	server      *WebSocketServer
	conn        *websocket.Conn
	id          string
	outChan     chan moleculer.Payload
	inChan      chan string
	receiveDone chan bool
	sendDone    chan bool

	name              string
	value             string
	onClose           func()
	closed            bool
	receiveMessage    func(*websocket.Conn) (moleculer.Payload, error)
	sendMessage       func(*websocket.Conn, moleculer.Payload) error
	prepareConnection func(*websocket.Conn)
	closeConn         func(*websocket.Conn)

	onStart []func(*WebSocketClient)
	onStop  []func(*WebSocketClient)
}

func receiveMessage(conn *websocket.Conn) (moleculer.Payload, error) {
	_, bts, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	return jsonSerializer.BytesToPayload(&bts), err
}

func sendMessage(conn *websocket.Conn, msg moleculer.Payload) error {
	return conn.WriteMessage(websocket.TextMessage, jsonSerializer.PayloadToBytes(msg))
}

func prepareConnection(conn *websocket.Conn) {
	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
}

func closeConn(conn *websocket.Conn) {
	if conn != nil {
		conn.Close()
	}
}

func newWebSocketClient(server *WebSocketServer, conn *websocket.Conn, id string) *WebSocketClient {
	return &WebSocketClient{
		server:            server,
		conn:              conn,
		id:                id,
		outChan:           make(chan moleculer.Payload),
		inChan:            make(chan string),
		receiveDone:       make(chan bool, 1),
		sendDone:          make(chan bool, 1),
		onClose:           func() {},
		closed:            false,
		receiveMessage:    receiveMessage,
		sendMessage:       sendMessage,
		prepareConnection: prepareConnection,
		closeConn:         closeConn,
	}
}

// pub publishes a message to the client.
func (wc *WebSocketClient) pub(topic string, p moleculer.Payload) {
	msg := payload.Empty().Add("topic", topic).Add("payload", p)
	go func() {
		wc.outChan <- msg
	}()
}

// start stats both receive and send pumps.
// it means that the client will start receiving and sending msgs.
func (wc *WebSocketClient) start() {
	wc.prepareConnection(wc.conn)
	go wc.receive()
	go wc.send()
	go wc.startHooks()
}

// stop both receive and send pumps.
//It means the client will longer send or receive any messages.
func (wc *WebSocketClient) stop() {
	wc.sendDone <- true
	wc.receiveDone <- true
	go wc.stoptHooks()
}

func (wc *WebSocketClient) startHooks() {
	for _, hk := range wc.onStart {
		hk(wc)
	}
}

func (wc *WebSocketClient) stoptHooks() {
	for _, hk := range wc.onStop {
		hk(wc)
	}
}

const (
	writeWait      = 10 * time.Second
	maxMessageSize = int64(1024)
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
)

func errorCountCheck(logger *log.Entry, maxErrors int) func(err error) bool {
	errorCount := 0
	return func(err error) bool {
		if err != nil {
			logger.Trace("Error sending/receiving msg - error: ", err)
			errorCount++
			if errorCount >= maxErrors {
				return true
			}
		}
		return false
	}
}

func (wc *WebSocketClient) close() {
	wc.closeConn(wc.conn)
}

//send loops while there is not a doneChan signal
//and on each look check if there is output in the outChan
// if there is sends a message down the websocket to the client.
func (wc *WebSocketClient) send() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		wc.close()
	}()
	shouldEnd := errorCountCheck(wc.server.context.Logger(), 5)
	for {
		select {
		case <-wc.sendDone:
			wc.onClose()
			return
		// send data to client
		case msg := <-wc.outChan:
			err := wc.sendMessage(wc.conn, msg)
			if shouldEnd(err) {
				return
			}
		case <-ticker.C:
			wc.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := wc.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// receive loops while there is not a doneChan signal
// and one each loop check if there is a JSON message on the
// websocket. where there is delivers to the message func.
func (wc *WebSocketClient) receive() {
	defer wc.closeConn(wc.conn)
	shouldEnd := errorCountCheck(wc.server.context.Logger(), 5)
	for {
		select {
		case <-wc.receiveDone:
			wc.onClose()
			return
		// read data from websocket connection
		default:
			msg, err := wc.receiveMessage(wc.conn)
			if shouldEnd(err) {
				return
			}
			if msg != nil && msg.Get("topic").Exists() {
				go wc.server.pub(wc, msg.Get("topic").String(), msg.Get("payload"))
			}
		}
	}
}

type WebSocketServer struct {
	clients       *sync.Map
	subscriptions *sync.Map
	context       moleculer.BrokerContext
}

func (ps *WebSocketServer) newClientConnection(conn *websocket.Conn) {
	ps.context.Logger().Info("websocket new connection")
	id := util.RandomString(12)
	client := newWebSocketClient(ps, conn, id)
	ps.clients.Store(id, client)
	go client.start()
}

// pub sends/publish a messages to the listener.
func (ps *WebSocketServer) pub(client *WebSocketClient, topic string, params moleculer.Payload) int {
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
func (ps *WebSocketServer) sub(topic string, handler subHandler) {
	temp, exist := ps.subscriptions.Load(topic)
	if !exist {
		ps.subscriptions.Store(topic, []subHandler{handler})
		return
	}
	ps.subscriptions.Store(topic, append(temp.([]subHandler), handler))
}

var upgrader = websocket.Upgrader{}

func (ps *WebSocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		ps.context.Logger().Error("Error on websocket.upgrade - error:", err)
		return
	}
	ps.newClientConnection(conn)
}

func newWebSocketServer(context moleculer.BrokerContext) *WebSocketServer {
	ps := &WebSocketServer{
		clients:       &sync.Map{},
		subscriptions: &sync.Map{},
		context:       context,
	}
	return ps
}

func isString(v interface{}) bool {
	_, ok := v.(string)
	return ok
}

// createWebSocketServer checks service settings and if enabled create an websockets server.
func createWebSocketServer(context moleculer.BrokerContext, settings map[string]interface{}) (*WebSocketServer, string) {
	if settings["websockets"] != nil && settings["websockets"] != "" && isString(settings["websockets"]) {
		path := settings["websockets"].(string)
		context.Logger().Debug("websockets settings found -> binding websockets on path: ", path)
		return newWebSocketServer(context), path
	}
	return nil, ""
}

type SocketMixin interface {
	// Init Initializes the Mixin.
	Init(moleculer.BrokerContext, *WebSocketServer)
}

type WebSocketMixin struct {
	Settings map[string]interface{}
	Mixins   []SocketMixin

	settings map[string]interface{}
}

var defaultSettings = map[string]interface{}{
	// websockets path where the server will listen for websocket connections.
	"websockets": "/ws/",
}

func (m *WebSocketMixin) ensureSettings() {
	if len(m.settings) == 0 {
		m.settings = service.MergeSettings(defaultSettings, m.Settings)
	}
}

func (m *WebSocketMixin) initMixins(context moleculer.BrokerContext, server *WebSocketServer) {
	// call mixins
	for _, mx := range m.Mixins {
		mx.Init(context, server)
	}
}

// RouterStarting create a websocket server and add to the mux.router
func (m *WebSocketMixin) RouterStarting(context moleculer.BrokerContext, router *mux.Router) {
	m.ensureSettings()
	context.Logger().Info("WebSocket Mixin -> RouterStarting() settings: ", m.settings)
	server, path := createWebSocketServer(context, m.settings)
	if server != nil {
		m.initMixins(context, server)
		router.Handle(path, server)
	}
}
