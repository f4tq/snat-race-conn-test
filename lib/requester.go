package lib

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"time"
	"net"
	"context"
	"strings"
)

const (
	// SlowRequestMs defines after how many millisecond a request is considered as slow and
	// shuold be logged
	SlowRequestMs = 500
)

// Requester performs HTTP requests at a pre-defined interval and logs error or slow
// response. It also writes the result in a channel for statistics
type Requester struct {
	stopCh     chan struct{}
	interval   int
	timeout    int
	url        string
	measureCh  chan int64
	resolve    string
	hostHeader string
}

func NewRequester(interval int, timeout int, url string, resolve string, measureCh chan int64) *Requester {
	return &Requester{
		stopCh:    make(chan struct{}),
		interval:  interval,
		timeout:   timeout,
		url:       url,
		measureCh: measureCh,
		resolve:   resolve,
	}
}

func (c *Requester) Run() {
	ticker := time.NewTicker(time.Duration(c.interval) * time.Microsecond)
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
	if len(c.resolve) > 0 {
		pieces := strings.Split(c.resolve, ":")

		// or create your own transport, there's an example on godoc.
		http.DefaultTransport.(*http.Transport).DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			fmt.Println("address original =", c.resolve)
			addr = fmt.Sprintf("%s:%s", pieces[2], pieces[1])
			fmt.Println("address modified =", addr)
			c.hostHeader = pieces[0]
			return dialer.DialContext(ctx, network, addr)
		}
	}
	for {
		select {
		case <-ticker.C:
			// Prepare request and http client
			requestID := rand.Int()
			urlWithID := fmt.Sprintf("%s?%d", c.url, requestID)
			client := http.Client{
				Timeout: time.Duration(c.timeout) * time.Millisecond,
			}

			// Fire the request
			requestStart := time.Now()
			req, err := http.NewRequest("GET", urlWithID, nil)
			if err != nil {
				panic(err)
			}
			if len(c.hostHeader) > 0{
				req.Header = map[string][]string{
					"Host": {c.hostHeader},
				}
			}
			response, err := client.Do(req)

			// Mesure time spent
			elapsed := time.Since(requestStart)
			c.measureCh <- elapsed.Nanoseconds()

			// Log error if any and abort
			if err != nil {
				fmt.Printf("%s | error | %6dms | %s\n", time.Now().Format("2006-01-02 15:04:05"), elapsed.Nanoseconds()/1000000, err)
				continue
			}

			// Log slow request
			if elapsed.Nanoseconds()/1000000 > SlowRequestMs {
				defer response.Body.Close()

				// Read response body
				bodyBytes, err := ioutil.ReadAll(response.Body)
				if err != nil {
					panic(err)
				}

				bodyString := string(bodyBytes)
				fmt.Printf("%s | slow | %6dms | %s | %d\n", time.Now().Format("2006-01-02 15:04:05"), elapsed.Nanoseconds()/1000000, bodyString, requestID)
			}

		case <-c.stopCh:
			ticker.Stop()
			return
		}
	}
}

func (c *Requester) Stop() {
	c.stopCh <- struct{}{}
}
