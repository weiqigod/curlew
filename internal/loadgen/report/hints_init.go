package report

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("loadgen/report",
		apierrors.RegisteredError{
			Name: "ErrUnsupportedFormat",
			Err:  ErrUnsupportedFormat,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInput, Code: "LOADGEN_REPORT_UNSUPPORTED_FORMAT", Hint: "Use a supported report format: json, html, or csv."},
		},
	)
}
