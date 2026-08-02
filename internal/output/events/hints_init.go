package events

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("output/events",
		apierrors.RegisteredError{
			Name: "ErrEmitterClosed",
			Err:  ErrEmitterClosed,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "EVENTS_EMITTER_CLOSED",
				Hint:     "Do not call Emit methods after Close has been called",
			},
		},
	)
}
