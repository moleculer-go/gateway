package gateway

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestMoleculerWeb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Unit Tests")
}
