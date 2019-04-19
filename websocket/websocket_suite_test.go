package websocket_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestWebsocket(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Websocket Suite")
}
