package notifier

import (
	"context"
	"fmt"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

type FCMNotifier struct {
	client *messaging.Client
}

func NewFCM(ctx context.Context, projectID string, credentialsPath string) (*FCMNotifier, error) {
	opts := []option.ClientOption{}
	if credentialsPath != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsPath))
	}
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID}, opts...)
	if err != nil {
		return nil, err
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, err
	}
	return &FCMNotifier{client: client}, nil
}

func (f *FCMNotifier) Send(ctx context.Context, payload Payload) error {
	msg := &messaging.Message{
		Topic: payload.Topic,
		Data: map[string]string{
			"courier_id":  payload.CourierID,
			"lat":         fmt.Sprintf("%f", payload.Lat),
			"lng":         fmt.Sprintf("%f", payload.Lng),
			"recorded_at": payload.Recorded.Format(time.RFC3339Nano),
		},
	}
	_, err := f.client.Send(ctx, msg)
	return err
}
