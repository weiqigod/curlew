package parser

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("parser",
		apierrors.RegisteredError{
			Name: "ErrFileNotFound",
			Err:  ErrFileNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_FILE_NOT_FOUND",
				Hint:     "Verify the path is correct and readable. Run from the project root or pass an absolute path.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidYAML",
			Err:  ErrInvalidYAML,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_INVALID_YAML",
				Hint:     "Fix the YAML syntax at the line indicated. Check indentation and quoting.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrEmptyCollection",
			Err:  ErrEmptyCollection,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_EMPTY_COLLECTION",
				Hint:     "Add a top-level name: field to the collection file.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnsupportedMethod",
			Err:  ErrUnsupportedMethod,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_UNSUPPORTED_METHOD",
				Hint:     "Use a standard HTTP method: GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrMissingRequiredField",
			Err:  ErrMissingRequiredField,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_MISSING_REQUIRED_FIELD",
				Hint:     "Add the missing field named in the message to the request definition.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrExternalFileNotFound",
			Err:  ErrExternalFileNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_EXTERNAL_FILE_NOT_FOUND",
				Hint:     "The file referenced by request_file: does not exist. Check the path relative to the collection file.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrCircularFileReference",
			Err:  ErrCircularFileReference,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_CIRCULAR_FILE_REFERENCE",
				Hint:     "Remove the circular reference: a request_file: must not transitively reference itself. Check the request_file: chain.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrMutuallyExclusive",
			Err:  ErrMutuallyExclusive,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_MUTUALLY_EXCLUSIVE",
				Hint:     "Remove one of the conflicting fields named in the message.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnsupportedProtocol",
			Err:  ErrUnsupportedProtocol,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_UNSUPPORTED_PROTOCOL",
				Hint:     "Use a supported protocol: http, https, graphql, or websocket.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidFieldValue",
			Err:  ErrInvalidFieldValue,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_INVALID_FIELD_VALUE",
				Hint:     "Correct the field value format described in the message.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrCircularInclude",
			Err:  ErrCircularInclude,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_CIRCULAR_INCLUDE",
				Hint:     "Remove the circular include: a collection may not transitively include itself. Check the include: chain and break the cycle.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrIncludeNotFound",
			Err:  ErrIncludeNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_INCLUDE_NOT_FOUND",
				Hint:     "Verify the include: path resolves relative to the including file.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrDuplicateRequestName",
			Err:  ErrDuplicateRequestName,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_DUPLICATE_REQUEST_NAME",
				Hint:     "Rename one of the duplicated requests. Main request names must be unique because --only, per-request markdown files, and CI report rows all address requests by name.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrSlugEmpty",
			Err:  ErrSlugEmpty,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_SLUG_EMPTY",
				Hint:     "Rename the request to include at least one ASCII letter or digit. Slugs are used as filenames for per-request markdown reports.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnknownDependsOn",
			Err:  ErrUnknownDependsOn,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_UNKNOWN_DEPENDS_ON",
				Hint:     "depends_on: must reference an item by its exact name (case-sensitive). Check that the name exists in the collection.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrCelAndOperatorMutuallyExclusive",
			Err:  ErrCelAndOperatorMutuallyExclusive,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_CEL_OPERATOR_EXCLUSIVE",
				Hint:     "A cel: assertion entry may not carry any operator key (e.g. eq:, equals:). Use either cel: or an operator assertion per entry, not both.",
			},
		},
	)
}
