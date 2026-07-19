package notify

import (
	"time"

	"github.com/go-resty/resty/v2"
)

const notificationHTTPTimeout = 15 * time.Second

func newNotificationHTTPClient() *resty.Client {
	return resty.New().SetTimeout(notificationHTTPTimeout)
}
