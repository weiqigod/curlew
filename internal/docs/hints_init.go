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
		apierrors.RegisteredError{
			Name: "ErrNoGateModes",
			Err:  ErrNoGateModes,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "DOCS_NO_GATE_MODES",
				Hint:     "scripts/ci-local.sh has no `case \"$MODE\" in` statement, or every arm in it is the help arm or the default arm. Check the script has not been restructured.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrNoGateInvocations",
			Err:  ErrNoGateInvocations,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "DOCS_NO_GATE_INVOCATIONS",
				Hint:     "The document names no `./scripts/ci-local.sh` invocation. Add at least one, e.g. `./scripts/ci-local.sh --go`.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrNoChecklist",
			Err:  ErrNoChecklist,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "DOCS_NO_CHECKLIST",
				Hint:     "The named heading either does not exist or carries no `- [ ]` items directly beneath it. Check the heading text matches exactly.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrNoModulePath",
			Err:  ErrNoModulePath,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "DOCS_NO_MODULE_PATH",
				Hint:     "go.mod has no `module <path>` directive, or the module is not hosted at github.com. Check go.mod has not been corrupted.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrNoFrontDoorFiles",
			Err:  ErrNoFrontDoorFiles,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "DOCS_NO_FRONT_DOOR_FILES",
				Hint:     "The FrontDoor registry (internal/docs/frontdoor.go) is empty. Restore its entries.",
			},
		},
	)
}
