package rate

import (
	"encoding/json"
	"sync/atomic"
	"time"
)

type Rate struct {
	bucketSize        int64
	bucketSurplusSize int64
	bucketAddSize     int64
	stopChan          chan bool
	NowRate           int64
}

func NewRate(addSize int64) *Rate {
	return &Rate{
		bucketSize:        addSize * 2,
		bucketSurplusSize: 0,
		bucketAddSize:     addSize,
		stopChan:          make(chan bool),
	}
}

func (s *Rate) Start() {
	go s.session()
}

func (s *Rate) add(size int64) {
	for {
		current := atomic.LoadInt64(&s.bucketSurplusSize)
		available := s.bucketSize - current
		if available <= 0 {
			return
		}
		addSize := size
		if addSize > available {
			addSize = available
		}
		if atomic.CompareAndSwapInt64(&s.bucketSurplusSize, current, current+addSize) {
			return
		}
	}
}

// 回桶
func (s *Rate) ReturnBucket(size int64) {
	s.add(size)
}

// 停止
func (s *Rate) Stop() {
	s.stopChan <- true
}

func (s *Rate) Get(size int64) {
	if s.take(size) {
		return
	}
	ticker := time.NewTicker(time.Millisecond * 100)
	for {
		select {
		case <-ticker.C:
			if s.take(size) {
				ticker.Stop()
				return
			}
		}
	}
}

func (s *Rate) take(size int64) bool {
	for {
		current := atomic.LoadInt64(&s.bucketSurplusSize)
		if current < size {
			return false
		}
		if atomic.CompareAndSwapInt64(&s.bucketSurplusSize, current, current-size) {
			return true
		}
	}
}

func (s *Rate) session() {
	ticker := time.NewTicker(time.Second * 1)
	for {
		select {
		case <-ticker.C:
			current := atomic.LoadInt64(&s.bucketSurplusSize)
			if rs := s.bucketAddSize - current; rs > 0 {
				atomic.StoreInt64(&s.NowRate, rs)
			} else {
				atomic.StoreInt64(&s.NowRate, s.bucketSize-current)
			}
			s.add(s.bucketAddSize)
		case <-s.stopChan:
			ticker.Stop()
			return
		}
	}
}

func (s *Rate) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		NowRate int64
	}{NowRate: atomic.LoadInt64(&s.NowRate)})
}
