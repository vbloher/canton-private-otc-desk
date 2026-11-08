package resend

import (
	"context"
	"net/http"

	resendsdk "github.com/resend/resend-go/v4"

	"github.com/vbloher/canton-private-otc-desk/pkg/otel/tracer"
)

func (r *Resend) Subscribe(ctx context.Context, email string) error {
	ctx, span := tracer.Start(ctx, "adapter resend Subscribe")
	defer span.End()

	update := &resendsdk.UpdateContactRequest{Email: email}
	update.SetUnsubscribed(false)

	_, err := r.client.Contacts.UpdateWithContext(ctx, update)

	switch {
	case statusIs(err, http.StatusNotFound):
		return r.create(ctx, email)
	case err != nil:
		return sanitize(err)
	}

	return r.addToSegment(ctx, email)
}

func (r *Resend) Unsubscribe(ctx context.Context, email string) error {
	ctx, span := tracer.Start(ctx, "adapter resend Unsubscribe")
	defer span.End()

	update := &resendsdk.UpdateContactRequest{Email: email}
	update.SetUnsubscribed(true)

	_, err := r.client.Contacts.UpdateWithContext(ctx, update)
	if err != nil && !statusIs(err, http.StatusNotFound) {
		return sanitize(err)
	}

	return nil
}

func (r *Resend) create(ctx context.Context, email string) error {
	contact := &resendsdk.CreateContactRequest{Email: email}

	if r.config.SegmentID != "" {
		contact.Segments = []resendsdk.ContactSegmentRef{{Id: r.config.SegmentID}}
	}

	_, err := r.client.Contacts.CreateWithContext(ctx, contact)
	if err != nil {
		return sanitize(err)
	}

	return nil
}

func (r *Resend) addToSegment(ctx context.Context, email string) error {
	if r.config.SegmentID == "" {
		return nil
	}

	_, err := r.client.Contacts.Segments.AddWithContext(ctx, &resendsdk.AddContactSegmentRequest{
		Email:     email,
		SegmentId: r.config.SegmentID,
	})
	if err != nil && !statusIs(err, http.StatusConflict) {
		return sanitize(err)
	}

	return nil
}
