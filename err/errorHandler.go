package e

import (
	"context"
	"github.com/pkg/errors"
)

func HTTPErrHandler(ctx context.Context, err error) *HttpErr {

	var httpErr *HttpErr
	var asHttpErr *HttpErr
	var asCodeErr *CodeErr

	if errors.As(err, &asHttpErr) {
		httpErr = asHttpErr
	} else if errors.As(err, &asCodeErr) {
		httpErr = NewHttpErr(asCodeErr, nil)
	} else {
		httpErr = NewHttpErr(FailedErr, err)
	}
	if httpErr.Level <= WarnLevel {
		SendMessage(ctx, err)
	}
	return httpErr
}
