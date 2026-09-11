package simulation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"ticketing-master/repository"
	reservationRepo "ticketing-master/repository/reservations"
	"ticketing-master/service"
	reservationService "ticketing-master/service/reservations"
)

type Transport string

const (
	// TransportInProcess calls the service layer directly. No sockets, no auth, no JSON,
	// so it can drive far more users than the machine has file descriptors — use it to
	// prove the oversell invariant at scale.
	TransportInProcess Transport = "inproc"
	// TransportHTTP goes through the real router: socket, middleware, JWT, JSON. Slower
	// and bounded by file descriptors, but its latencies are the ones a user would feel.
	TransportHTTP Transport = "http"
)

// queueStatus is one poll's worth of answer, identical in shape across both transports so
// the engine never has to know which one it is driving.
type queueStatus struct {
	WhitelistTTL time.Duration
	Alive        bool
	SoldOut      bool
}

type client interface {
	Enqueue(ctx context.Context, userId string) (time.Duration, error)
	Poll(ctx context.Context, userId string) (queueStatus, error)
	Reserve(ctx context.Context, userId string, quantity uint32) error
	ReleaseSlot(ctx context.Context, userId string) error
}

type inProcessClient struct {
	services     *service.Services
	repositories *repository.Repositories
	eventId      string
}

func (c *inProcessClient) Enqueue(ctx context.Context, userId string) (time.Duration, error) {
	return c.services.VirtualQueueService.Enqueue(ctx, c.eventId, userId)
}

func (c *inProcessClient) Poll(ctx context.Context, userId string) (queueStatus, error) {
	status, err := c.services.VirtualQueueService.Ping(ctx, c.eventId, userId)
	if err != nil {
		return queueStatus{}, err
	}

	return queueStatus(status), nil
}

func (c *inProcessClient) Reserve(ctx context.Context, userId string, quantity uint32) error {
	_, err := c.services.ReservationService.ReserveEvent(ctx, &reservationService.ReserveEventRequest{
		EventId:  c.eventId,
		UserId:   userId,
		Quantity: quantity,
	})

	return err
}

func (c *inProcessClient) ReleaseSlot(ctx context.Context, userId string) error {
	return c.services.VirtualQueueService.Dequeue(ctx, c.eventId, userId)
}

type httpClient struct {
	baseURL string
	eventId string
	tokens  map[string]string
	http    *http.Client
}

func newHTTPClient(baseURL string, eventId string, tokens map[string]string, workers int) *httpClient {
	// Every simulated user is an independent connection, so the default transport's
	// 2-per-host idle cap would serialise them onto a handful of sockets and measure
	// connection churn instead of the server.
	maxConns := workers * 2
	if maxConns < 100 {
		maxConns = 100
	}

	return &httpClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		eventId: eventId,
		tokens:  tokens,
		http: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        maxConns,
				MaxIdleConnsPerHost: maxConns,
				MaxConnsPerHost:     0,
				IdleConnTimeout:     90 * time.Second,
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
		},
	}
}

type httpError struct {
	Status  int
	Message string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("http %d: %s", e.Status, e.Message)
}

func (c *httpClient) do(ctx context.Context, method string, path string, userId string, body any, out any) (int, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token, ok := c.tokens[userId]; ok {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, err
	}

	if res.StatusCode >= 400 {
		var errBody struct {
			Error string `json:"error"`
		}
		json.Unmarshal(raw, &errBody)
		return res.StatusCode, &httpError{Status: res.StatusCode, Message: errBody.Error}
	}

	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return res.StatusCode, err
		}
	}

	return res.StatusCode, nil
}

func (c *httpClient) Enqueue(ctx context.Context, userId string) (time.Duration, error) {
	var out struct {
		Status string `json:"status"`
	}
	_, err := c.do(ctx, http.MethodPost, "/virtual-queues/enqueue", userId,
		map[string]string{"eventId": c.eventId}, &out)
	if err != nil {
		return 0, err
	}

	// The enqueue response reports admission as a state, not a TTL. The engine only
	// checks the sign, and the first poll supplies the real value.
	if out.Status == "whitelisted" {
		return time.Second, nil
	}

	return 0, nil
}

func (c *httpClient) Poll(ctx context.Context, userId string) (queueStatus, error) {
	var out struct {
		Admitted   bool  `json:"admitted"`
		TTLSeconds int64 `json:"ttlSeconds"`
		SoldOut    bool  `json:"soldOut"`
	}
	status, err := c.do(ctx, http.MethodPost, "/virtual-queues/ping", userId,
		map[string]string{"eventId": c.eventId}, &out)
	if err != nil {
		// 404 is the server saying this client is neither whitelisted nor alive, which is
		// a state the engine counts rather than an error it reports.
		if status == http.StatusNotFound {
			return queueStatus{Alive: false}, nil
		}
		return queueStatus{}, err
	}

	ttl := time.Duration(out.TTLSeconds) * time.Second
	if out.Admitted && ttl <= 0 {
		ttl = time.Second
	}

	return queueStatus{WhitelistTTL: ttl, Alive: true, SoldOut: out.SoldOut}, nil
}

func (c *httpClient) Reserve(ctx context.Context, userId string, quantity uint32) error {
	_, err := c.do(ctx, http.MethodPost, "/events/"+c.eventId+"/reserve", userId,
		map[string]any{"quantity": quantity}, nil)

	return translateReserveError(err)
}

func (c *httpClient) ReleaseSlot(ctx context.Context, userId string) error {
	_, err := c.do(ctx, http.MethodPost, "/virtual-queues/dequeue", userId,
		map[string]string{"eventId": c.eventId}, nil)

	return err
}

// translateReserveError maps the wire response back onto the sentinels the engine
// classifies by, so both transports produce the same breakdown.
func translateReserveError(err error) error {
	var httpErr *httpError
	if !errors.As(err, &httpErr) {
		return err
	}
	if httpErr.Status != http.StatusConflict {
		return err
	}

	switch httpErr.Message {
	case reservationRepo.ErrInsufficientCapacity.Error():
		return reservationRepo.ErrInsufficientCapacity
	case reservationRepo.ErrExceedMaxReserveQuantity.Error():
		return reservationRepo.ErrExceedMaxReserveQuantity
	}

	return err
}

var _ client = (*inProcessClient)(nil)
var _ client = (*httpClient)(nil)
