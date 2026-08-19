package docs

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage(
		"docs",
		apierrors.RegisteredError{
			Name: "ErrNoLayoutTable",
			Err:  ErrNoLayoutTable,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "DOCS_NO_LAYOUT_TABLE",
				Hint:     "README.md has no table under the \"Repository layout\" heading with a Directory | What it is | Relationship to the CLI header, or that table has zero rows. Add or restore it.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrNoTopLevelDirs",
			Err:  ErrNoTopLevelDirs,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "DOCS_NO_TOP_LEVEL_DIRS",
				Hint:     "git ls-files reported zero top-level directories for the given root. Check that root points at a real checkout of this repository.",
			},
		},
	)
}
