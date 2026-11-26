package main

import (
	"net/url"
	"testing"
)

func TestBuildMigrateDatabaseURL(t *testing.T) {
	tests := []struct {
		name           string
		uri            string
		dbName         string
		wantErr        bool
		wantScheme     string
		wantHost       string
		wantPath       string
		wantAuthSource string
		wantParams     map[string]string
	}{
		{
			name:           "standard uri without trailing slash",
			uri:            "mongodb://localhost:27017",
			dbName:         "opspulse",
			wantScheme:     "mongodb",
			wantHost:       "localhost:27017",
			wantPath:       "/opspulse",
			wantAuthSource: "admin",
		},
		{
			name:           "uri with trailing slash",
			uri:            "mongodb://localhost:27017/",
			dbName:         "opspulse",
			wantScheme:     "mongodb",
			wantHost:       "localhost:27017",
			wantPath:       "/opspulse",
			wantAuthSource: "admin",
		},
		{
			name:           "uri with trailing slash and query parameter",
			uri:            "mongodb://localhost:27017/?directConnection=true",
			dbName:         "opspulse",
			wantScheme:     "mongodb",
			wantHost:       "localhost:27017",
			wantPath:       "/opspulse",
			wantAuthSource: "admin",
			wantParams:     map[string]string{"directConnection": "true"},
		},
		{
			name:           "uri with existing db path and query parameter",
			uri:            "mongodb://localhost:27017/mydb?retryWrites=true",
			dbName:         "opspulse",
			wantScheme:     "mongodb",
			wantHost:       "localhost:27017",
			wantPath:       "/opspulse",
			wantAuthSource: "admin",
			wantParams:     map[string]string{"retryWrites": "true"},
		},
		{
			name:           "uri with existing authSource is not duplicated",
			uri:            "mongodb://user:pass@localhost:27017/mydb?authSource=custom&retryWrites=true",
			dbName:         "mydb",
			wantScheme:     "mongodb",
			wantHost:       "localhost:27017",
			wantPath:       "/mydb",
			wantAuthSource: "custom",
			wantParams:     map[string]string{"retryWrites": "true"},
		},
		{
			name:           "mongodb+srv uri preserves host and query params",
			uri:            "mongodb+srv://user:pass@cluster.example.net/?retryWrites=true&w=majority",
			dbName:         "opspulse",
			wantScheme:     "mongodb+srv",
			wantHost:       "cluster.example.net",
			wantPath:       "/opspulse",
			wantAuthSource: "admin",
			wantParams:     map[string]string{"retryWrites": "true", "w": "majority"},
		},
		{
			name:           "replica set multi-host uri preserves hosts and query params",
			uri:            "mongodb://mongo1:27017,mongo2:27017/?replicaSet=rs0&directConnection=false",
			dbName:         "opspulse",
			wantScheme:     "mongodb",
			wantHost:       "mongo1:27017,mongo2:27017",
			wantPath:       "/opspulse",
			wantAuthSource: "admin",
			wantParams:     map[string]string{"replicaSet": "rs0", "directConnection": "false"},
		},
		{
			name:           "empty dbName falls back to existing path dbName",
			uri:            "mongodb://localhost:27017/mydb?retryWrites=true",
			dbName:         "",
			wantScheme:     "mongodb",
			wantHost:       "localhost:27017",
			wantPath:       "/mydb",
			wantAuthSource: "admin",
			wantParams:     map[string]string{"retryWrites": "true"},
		},
		{
			name:    "empty uri returns error",
			uri:     "",
			dbName:  "opspulse",
			wantErr: true,
		},
		{
			name:    "missing dbName and empty uri path returns error",
			uri:     "mongodb://localhost:27017/",
			dbName:  "",
			wantErr: true,
		},
		{
			name:    "unsupported scheme returns error",
			uri:     "postgres://localhost:5432",
			dbName:  "opspulse",
			wantErr: true,
		},
		{
			name:    "malformed uri returns error",
			uri:     "://bad-uri",
			dbName:  "opspulse",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildMigrateDatabaseURL(tc.uri, tc.dbName)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (url=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			parsed, err := url.Parse(got)
			if err != nil {
				t.Fatalf("output URL %q failed url.Parse: %v", got, err)
			}
			if parsed.Scheme != tc.wantScheme {
				t.Errorf("expected scheme %q, got %q", tc.wantScheme, parsed.Scheme)
			}
			if parsed.Host != tc.wantHost {
				t.Errorf("expected host %q, got %q", tc.wantHost, parsed.Host)
			}
			if parsed.Path != tc.wantPath {
				t.Errorf("expected path %q, got %q", tc.wantPath, parsed.Path)
			}
			q := parsed.Query()
			if len(q["authSource"]) != 1 || q.Get("authSource") != tc.wantAuthSource {
				t.Errorf("expected single authSource=%q, got %v", tc.wantAuthSource, q["authSource"])
			}
			for k, wantVal := range tc.wantParams {
				if q.Get(k) != wantVal {
					t.Errorf("expected query param %s=%q, got %q", k, wantVal, q.Get(k))
				}
			}
		})
	}
}
