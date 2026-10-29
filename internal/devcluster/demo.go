package devcluster

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Demo seeds streams, consumers, KV and object store data in the APP
// account and keeps generating traffic until ctx is cancelled, so every
// natsmin page has something to show.
func Demo(ctx context.Context, urls []string) error {
	nc, err := nats.Connect(strings.Join(urls, ","), nats.UserInfo(AppUser, AppPass), nats.Name("natsmin demo"))
	if err != nil {
		return err
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		return err
	}

	streams := []jetstream.StreamConfig{
		{Name: "ORDERS", Subjects: []string{"orders.>"}, Replicas: 3, MaxAge: 24 * time.Hour, Description: "Order lifecycle events", Placement: &jetstream.Placement{Cluster: "east"}},
		{Name: "PAYMENTS", Subjects: []string{"payments.>"}, Replicas: 3, Retention: jetstream.WorkQueuePolicy, Description: "Payment jobs", Placement: &jetstream.Placement{Cluster: "west"}},
		{Name: "AUDIT", Subjects: []string{"audit.>"}, Replicas: 1, Storage: jetstream.MemoryStorage, MaxMsgs: 5000, Description: "Recent audit events"},
	}
	for _, cfg := range streams {
		if _, err := js.CreateOrUpdateStream(ctx, cfg); err != nil {
			return fmt.Errorf("stream %s: %w", cfg.Name, err)
		}
	}
	consumers := map[string]jetstream.ConsumerConfig{
		"ORDERS":   {Durable: "fulfilment", AckPolicy: jetstream.AckExplicitPolicy, FilterSubject: "orders.*.created", Description: "Ships created orders"},
		"PAYMENTS": {Durable: "processor", AckPolicy: jetstream.AckExplicitPolicy, MaxAckPending: 100},
	}
	for stream, cfg := range consumers {
		if _, err := js.CreateOrUpdateConsumer(ctx, stream, cfg); err != nil {
			return fmt.Errorf("consumer %s: %w", cfg.Durable, err)
		}
	}
	analytics := jetstream.ConsumerConfig{Durable: "analytics", AckPolicy: jetstream.AckExplicitPolicy, Description: "Lags behind on purpose"}
	if _, err := js.CreateOrUpdateConsumer(ctx, "ORDERS", analytics); err != nil {
		return err
	}

	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "config", History: 10, Replicas: 3, Description: "Service configuration"})
	if err != nil {
		return err
	}
	for k, v := range map[string]string{
		"checkout.enabled":  "true",
		"checkout.timeout":  "30s",
		"pricing.currency":  "EUR",
		"features.newcart":  `{"rollout": 0.25, "regions": ["eu", "us"]}`,
		"service.api.limit": "1000",
	} {
		if _, err := kv.Put(ctx, k, []byte(v)); err != nil {
			return err
		}
	}
	obj, err := js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: "assets", Description: "Static files"})
	if err != nil {
		return err
	}
	if _, err := obj.PutString(ctx, "readme.txt", "natsmin demo object store\n"); err != nil {
		return err
	}
	if _, err := obj.PutBytes(ctx, "data/sample.bin", make([]byte, 3<<20)); err != nil {
		return err
	}

	// A request/reply service.
	if _, err := nc.QueueSubscribe("svc.pricing.quote", "pricing", func(m *nats.Msg) {
		_ = m.Respond([]byte(fmt.Sprintf(`{"price": %.2f, "currency": "EUR"}`, 10+rand.Float64()*90)))
	}); err != nil {
		return err
	}

	// Workers that ack some messages.
	fulfil, _ := js.Consumer(ctx, "ORDERS", "fulfilment")
	fcc, err := fulfil.Consume(func(m jetstream.Msg) { _ = m.Ack() })
	if err == nil {
		defer fcc.Stop()
	}
	proc, _ := js.Consumer(ctx, "PAYMENTS", "processor")
	pcc, err := proc.Consume(func(m jetstream.Msg) {
		if rand.IntN(20) == 0 {
			_ = m.Nak()
			return
		}
		_ = m.Ack()
	})
	if err == nil {
		defer pcc.Stop()
	}

	regions := []string{"eu", "us", "apac"}
	events := []string{"created", "paid", "shipped", "delivered"}
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	n := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		n++
		region := regions[rand.IntN(len(regions))]
		ev := events[rand.IntN(len(events))]
		body, _ := json.Marshal(map[string]any{"id": n, "region": region, "event": ev, "amount": rand.IntN(50000) / 100.0, "at": time.Now()})
		_, _ = js.PublishAsync(fmt.Sprintf("orders.%s.%s", region, ev), body)
		if n%3 == 0 {
			_, _ = js.PublishAsync("payments."+region, body)
		}
		if n%5 == 0 {
			_ = nc.Publish("audit.login", []byte(fmt.Sprintf(`{"user":"u%d"}`, rand.IntN(100))))
		}
		if n%10 == 0 {
			_ = nc.Publish(fmt.Sprintf("telemetry.%s.cpu", region), []byte(fmt.Sprintf("%.1f", rand.Float64()*100)))
			_, _ = nc.Request("svc.pricing.quote", []byte(`{"sku":"A1"}`), time.Second)
		}
		if n%250 == 0 {
			_, _ = kv.Put(ctx, "checkout.timeout", []byte(fmt.Sprintf("%ds", 20+rand.IntN(20))))
		}
	}
}
