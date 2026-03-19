package cloudflare

import (
	"fmt"

	cloudflareapi "github.com/cloudflare/cloudflare-go/v6"
	cfdns "github.com/cloudflare/cloudflare-go/v6/dns"

	"dns-update/internal/provider"
)

// RecordOptions returns the desired shared provider options for Cloudflare-managed records.
func (c Config) RecordOptions() provider.RecordOptions {
	return recordOptions(c.Proxied)
}

func buildBatchParams(zoneID string, plan provider.Plan) (cfdns.RecordBatchParams, error) {
	deletes := make([]cfdns.RecordBatchParamsDelete, 0, len(plan.Operations))
	patches := make([]cfdns.BatchPatchUnionParam, 0, len(plan.Operations))
	posts := make([]cfdns.RecordBatchParamsPostUnion, 0, len(plan.Operations))

	for _, operation := range plan.Operations {
		switch operation.Kind {
		case provider.OperationDelete:
			deletes = append(deletes, cfdns.RecordBatchParamsDelete{
				ID: cloudflareapi.F(operation.Current.ID),
			})
		case provider.OperationUpdate:
			patch, err := toBatchPatch(operation.Current.ID, operation.Desired)
			if err != nil {
				return cfdns.RecordBatchParams{}, err
			}
			patches = append(patches, patch)
		case provider.OperationCreate:
			post, err := toRecordPost(operation.Desired)
			if err != nil {
				return cfdns.RecordBatchParams{}, err
			}
			posts = append(posts, post)
		default:
			return cfdns.RecordBatchParams{}, fmt.Errorf("unsupported operation kind %q", operation.Kind)
		}
	}

	params := cfdns.RecordBatchParams{
		ZoneID: cloudflareapi.F(zoneID),
	}
	if len(deletes) > 0 {
		params.Deletes = cloudflareapi.F(deletes)
	}
	if len(patches) > 0 {
		params.Patches = cloudflareapi.F(patches)
	}
	if len(posts) > 0 {
		params.Posts = cloudflareapi.F(posts)
	}
	return params, nil
}

func toRecordPost(record provider.Record) (cfdns.RecordBatchParamsPostUnion, error) {
	switch record.Type {
	case provider.RecordTypeA:
		params := cfdns.ARecordParam{
			Name:    cloudflareapi.F(normalizeAPIName(record.Name)),
			TTL:     cloudflareapi.F(cfdns.TTL(record.TTLSeconds)),
			Type:    cloudflareapi.F(cfdns.ARecordTypeA),
			Content: cloudflareapi.F(record.Content),
		}
		if record.Options.Proxy != nil {
			params.Proxied = cloudflareapi.F(*record.Options.Proxy)
		}
		return params, nil
	case provider.RecordTypeAAAA:
		params := cfdns.AAAARecordParam{
			Name:    cloudflareapi.F(normalizeAPIName(record.Name)),
			TTL:     cloudflareapi.F(cfdns.TTL(record.TTLSeconds)),
			Type:    cloudflareapi.F(cfdns.AAAARecordTypeAAAA),
			Content: cloudflareapi.F(record.Content),
		}
		if record.Options.Proxy != nil {
			params.Proxied = cloudflareapi.F(*record.Options.Proxy)
		}
		return params, nil
	default:
		return nil, fmt.Errorf("unsupported record type %q", record.Type)
	}
}

func toBatchPatch(id string, record provider.Record) (cfdns.BatchPatchUnionParam, error) {
	switch record.Type {
	case provider.RecordTypeA:
		params := cfdns.BatchPatchARecordParam{
			ID: cloudflareapi.F(id),
			ARecordParam: cfdns.ARecordParam{
				Name:    cloudflareapi.F(normalizeAPIName(record.Name)),
				TTL:     cloudflareapi.F(cfdns.TTL(record.TTLSeconds)),
				Type:    cloudflareapi.F(cfdns.ARecordTypeA),
				Content: cloudflareapi.F(record.Content),
			},
		}
		if record.Options.Proxy != nil {
			params.Proxied = cloudflareapi.F(*record.Options.Proxy)
		}
		return params, nil
	case provider.RecordTypeAAAA:
		params := cfdns.BatchPatchAAAARecordParam{
			ID: cloudflareapi.F(id),
			AAAARecordParam: cfdns.AAAARecordParam{
				Name:    cloudflareapi.F(normalizeAPIName(record.Name)),
				TTL:     cloudflareapi.F(cfdns.TTL(record.TTLSeconds)),
				Type:    cloudflareapi.F(cfdns.AAAARecordTypeAAAA),
				Content: cloudflareapi.F(record.Content),
			},
		}
		if record.Options.Proxy != nil {
			params.Proxied = cloudflareapi.F(*record.Options.Proxy)
		}
		return params, nil
	default:
		return nil, fmt.Errorf("unsupported record type %q", record.Type)
	}
}

func normalizeAPIName(name string) string {
	return provider.NormalizeName(name)
}

func normalizeProviderName(name string) string {
	return provider.NormalizeName(name) + "."
}

func recordOptions(proxy bool) provider.RecordOptions {
	value := proxy
	return provider.RecordOptions{Proxy: &value}
}
