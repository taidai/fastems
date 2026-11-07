package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/simpleiot/simpleiot/data"
)

// PointHistory reads existing JetStream history without creating another
// datastore. Results are bounded and ordered by the point's original time.
func (st *Store) PointHistory(nodeID, typ, key string, start, end time.Time, limit int) ([]data.Point, error) {
	return st.db.pointHistory(nodeID, typ, key, start, end, limit)
}

func (db *DbJetStream) pointHistory(nodeID, typ, key string, start, end time.Time, limit int) ([]data.Point, error) {
	if limit <= 0 || limit > 2000 {
		return nil, fmt.Errorf("history limit must be between 1 and 2000")
	}
	if !end.IsZero() && !start.IsZero() && end.Before(start) {
		return nil, fmt.Errorf("history end precedes start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	boundary := db.edgeCache.OwningBoundary(nodeID, db.meta.RootID)

	db.streamMu.Lock()
	streams := make([]jetstream.Stream, 0, len(db.streams))
	for _, stream := range db.streams {
		b, _, ok := streamBoundaryOrigin(stream.CachedInfo().Config)
		if ok && b == boundary {
			streams = append(streams, stream)
		}
	}
	db.streamMu.Unlock()

	result := []data.Point{}
	for _, stream := range streams {
		_, origin, ok := streamBoundaryOrigin(stream.CachedInfo().Config)
		if !ok {
			continue
		}
		cfg := jetstream.OrderedConsumerConfig{
			FilterSubjects:    []string{nodePointSubject(boundary, origin, nodeID, typ, key)},
			DeliverPolicy:     jetstream.DeliverAllPolicy,
			InactiveThreshold: 5 * time.Second,
		}
		if !start.IsZero() {
			cfg.DeliverPolicy = jetstream.DeliverByStartTimePolicy
			cfg.OptStartTime = &start
		}
		consumer, err := stream.OrderedConsumer(ctx, cfg)
		if err != nil {
			return nil, err
		}
		for {
			batch, err := consumer.Fetch(256, jetstream.FetchMaxWait(50*time.Millisecond))
			if err != nil {
				if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
					break
				}
				return nil, err
			}
			count := 0
			for message := range batch.Messages() {
				count++
				points, err := data.DecodePoints(message.Data())
				if err != nil {
					return nil, err
				}
				for _, point := range points {
					if (!start.IsZero() && point.Time.Before(start)) || (!end.IsZero() && point.Time.After(end)) {
						continue
					}
					result = append(result, point)
				}
			}
			if count == 0 {
				break
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Time.Equal(result[j].Time) {
			return result[i].Origin < result[j].Origin
		}
		return result[i].Time.Before(result[j].Time)
	})
	if len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result, nil
}
