package outbox_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vbloher/canton-private-otc-desk/pkg/outbox"
)

func Test_NewRelay_RejectsConfigThatCouldNeverPublish(t *testing.T) {
	valid := outbox.Config{Interval: time.Second, BatchSize: 1, Retention: time.Hour}

	cases := map[string]func(*outbox.Config){
		"no interval":  func(c *outbox.Config) { c.Interval = 0 },
		"empty batch":  func(c *outbox.Config) { c.BatchSize = 0 },
		"no retention": func(c *outbox.Config) { c.Retention = -time.Hour },
	}

	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			c := valid
			change(&c)

			_, err := outbox.NewRelay(c, nil, nil)
			require.ErrorIs(t, err, outbox.ErrInvalidConfig)
		})
	}
}
