package postgres_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
	"time"
)

func TestPostgreSQL(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	RegisterFailHandler(Fail)
	cfg, rep := GinkgoConfiguration()
	if d, ok := t.Deadline(); ok {
		cfg.Timeout = d.Sub(time.Now())
	}
	RunSpecs(t, "PostgreSQL", cfg, rep)
}
