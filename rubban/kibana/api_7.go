package kibana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/sherifabdlnaby/rubban/config"
	"github.com/sherifabdlnaby/rubban/log"
	"github.com/sherifabdlnaby/rubban/rubban/utils"
)

//APIVer7 Implements API Calls compatible with Kibana 7^
type APIVer7 struct {
	client *Client
	log    log.Logger
}

//NewAPIVer7 Constructor
func NewAPIVer7(config config.Kibana, log log.Logger) (*APIVer7, error) {
	client, err := NewKibanaClient(config, log.Extend("Client"))
	if err != nil {
		return &APIVer7{}, err
	}

	return &APIVer7{
		client: client,
		log:    log,
	}, nil
}

//Info Return Kibana Info
func (a *APIVer7) Info(ctx context.Context) (Info, error) {
	resp, err := a.client.Get(ctx, "/api/status", nil)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()

	info := Info{}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		err := json.NewDecoder(resp.Body).Decode(&info)
		if err != nil {
			return Info{}, err
		}
	}

	return info, err
}

//Indices Get Indices match supported filter (support wildcards)
func (a *APIVer7) Indices(ctx context.Context, filter string) ([]Index, error) {
	indices := make([]Index, 0)
	resp, err := a.client.Post(ctx, fmt.Sprintf("/api/console/proxy?path=_cat/indices/%s?format=json&h=index&method=GET", filter), nil)
	if err != nil {
		return indices, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		err := json.NewDecoder(resp.Body).Decode(&indices)
		if err != nil {
			return nil, err
		}
	}
	return indices, err
}

type FindIndexPatternResponse struct {
	Hits struct {
		Hits []struct {
			ID     string `json:"_id"`
			Source struct {
				IndexPattern struct {
					Title         string `json:"title"`
					TimeFieldName string `json:"timeFieldName"`
				} `json:"index-pattern"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

var alphaNumericRegex = regexp.MustCompile("[^a-zA-Z0-9]+")
var space = regexp.MustCompile(`\s+`)
var idxPatternID = regexp.MustCompile(`(index-pattern:)(.*)`)

//IndexPatterns Get IndexPatterns from kibana matching the supplied filter (support wildcards)
func (a *APIVer7) IndexPatterns(ctx context.Context, filter string, fields []string) ([]IndexPattern, error) {

	var IndexPatterns = make([]IndexPattern, 0)

	// Remove Non Alpha Numeric Chars AND Trim Duplicate Whitespaces.
	// As Index Pattern Names in Kibana Index is of type text. //TODO Explain why we did it this way.
	indexPatternTrimd := space.ReplaceAllString(alphaNumericRegex.ReplaceAllString(filter, " "), " ")

	requestBody := fmt.Sprintf(`{
	  "_source": ["index-pattern.title","index-pattern.timeFieldName"],
      "size": 10000,
	  "query": {
			"bool": {
		  "must": [
			{
			  "query_string" : {
				"query" : "%s",
				"auto_generate_synonyms_phrase_query": true,
				"analyze_wildcard": true,
				"default_operator": "AND"
				, "fields": ["index-pattern.title"]
				, "fuzziness": 0.0
				, "phrase_slop": 0
			}
			},
			{
			  "match_phrase": {
				"type": {
				  "query": "index-pattern"
				}
			  }
			}
		  ],
		  "filter": [],
		  "should": [],
		  "must_not": []
		}
	  }
	}`, indexPatternTrimd)

	resp, err := a.client.Post(ctx, "/api/console/proxy?path=.kibana/_search&method=POST", strings.NewReader(requestBody))
	if err != nil {
		return IndexPatterns, err
	}
	defer resp.Body.Close()

	response := FindIndexPatternResponse{}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		err := json.NewDecoder(resp.Body).Decode(&response)
		if err != nil {
			return nil, err
		}
	}

	regex := regexp.MustCompile(utils.ReplacerForRegex(filter))

	for _, hit := range response.Hits.Hits {
		if regex.MatchString(hit.Source.IndexPattern.Title) {
			IndexPatterns = append(IndexPatterns, IndexPattern{
				ID:            idxPatternID.ReplaceAllString(hit.ID, "$2"),
				Title:         hit.Source.IndexPattern.Title,
				TimeFieldName: hit.Source.IndexPattern.TimeFieldName,
			})
		}
	}
	return IndexPatterns, err
}

//BulkCreateIndexPattern Add Index Patterns to Kibana
func (a *APIVer7) BulkCreateIndexPattern(ctx context.Context, indexPattern []IndexPattern) error {
	if len(indexPattern) == 0 {
		return nil
	}

	// Prepare Requests
	bulkRequest := make([]BulkIndexPattern, 0)
	for _, pattern := range indexPattern {
		bulkRequest = append(bulkRequest, BulkIndexPattern{
			Type: "index-pattern",
			ID:   pattern.ID,
			Attributes: IndexPattern{
				Title:         pattern.Title,
				TimeFieldName: pattern.TimeFieldName,
			},
		})
	}

	// Json Marshaling
	buff, err := json.Marshal(bulkRequest)
	if err != nil {
		return fmt.Errorf("failed to JSON marshaling bulk create index pattern")
	}

	// Send Request
	resp, err := a.client.Post(ctx, "/api/saved_objects/_bulk_create?overwrite=true", bytes.NewReader(buff))
	if err != nil {
		return fmt.Errorf("failed to bulk create saved objects, error: %s", err.Error())
	}

	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to bulk create saved objects, error: %s", resp.Status)
	}

	return nil
}

type PutIndexPatternAttr struct {
	Title  string `json:"title"`
	Fields string `json:"fields,squash"`
}

type PutIndexPatternBody struct {
	Attributes PutIndexPatternAttr `json:"attributes"`
	Version    string              `json:"version"`
}

func (a *APIVer7) PutIndexPattern(ctx context.Context, indexPattern IndexPattern) error {

	return nil
}
