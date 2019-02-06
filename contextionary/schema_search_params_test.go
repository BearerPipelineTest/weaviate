package contextionary

import (
	"errors"
	"testing"

	"github.com/creativesoftwarefdn/weaviate/models"
	"github.com/stretchr/testify/assert"
)

func Test__SchemaSearch_Validation(t *testing.T) {
	tests := schemaSearchTests{
		{
			name: "valid params",
			searchParams: SearchParams{
				SearchType: SearchTypeClass,
				Name:       "foo",
				Certainty:  1.0,
			},
			expectedError: nil,
		},
		{
			name: "missing search name",
			searchParams: SearchParams{
				SearchType: SearchTypeClass,
				Name:       "",
				Certainty:  0.0,
			},
			expectedError: errors.New("Name cannot be empty"),
		},
		{
			name: "certainty too low",
			searchParams: SearchParams{
				SearchType: SearchTypeClass,
				Name:       "bestName",
				Certainty:  -4,
			},
			expectedError: errors.New("invalid Certainty: must be between 0 and 1, but got '-4.000000'"),
		},
		{
			name: "certainty too high",
			searchParams: SearchParams{
				SearchType: SearchTypeClass,
				Name:       "bestName",
				Certainty:  4,
			},
			expectedError: errors.New("invalid Certainty: must be between 0 and 1, but got '4.000000'"),
		},
		{
			name: "inavlid search type",
			searchParams: SearchParams{
				SearchType: SearchType("invalid"),
				Name:       "bestName",
				Certainty:  0.5,
			},
			expectedError: errors.New("SearchType must be SearchTypeClass or SearchTypeProperty, but got 'invalid'"),
		},
		{
			name: "valid keywords",
			searchParams: SearchParams{
				SearchType: SearchTypeClass,
				Name:       "foo",
				Certainty:  1.0,
				Keywords: models.SemanticSchemaKeywords{{
					Keyword: "foobar",
					Weight:  1.0,
				}},
			},
			expectedError: nil,
		},
		{
			name: "keywords with empty names",
			searchParams: SearchParams{
				SearchType: SearchTypeClass,
				Name:       "foo",
				Certainty:  1.0,
				Keywords: models.SemanticSchemaKeywords{{
					Keyword: "",
					Weight:  1.0,
				}},
			},
			expectedError: errors.New("invalid keyword at position 0: Keyword cannot be empty"),
		},
		{
			name: "keywords with invalid weights",
			searchParams: SearchParams{
				SearchType: SearchTypeClass,
				Name:       "foo",
				Certainty:  1.0,
				Keywords: models.SemanticSchemaKeywords{{
					Keyword: "bestkeyword",
					Weight:  1.3,
				}},
			},
			expectedError: errors.New("invalid keyword at position 0: invalid Weight: " +
				"must be between 0 and 1, but got '1.300000'"),
		},
	}

	tests.AssertValidation(t)
}

func (s schemaSearchTests) AssertValidation(t *testing.T) {
	for _, test := range s {
		t.Run(test.name, func(t *testing.T) {
			err := test.searchParams.Validate()

			// assert error
			assert.Equal(t, test.expectedError, err, "should match the expected error")

		})
	}
}