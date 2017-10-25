package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	gke "google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"
)

const (
	userAgent           = "spotter"
	serviceAccountType  = "service_account"
	gceURLSchema        = "https"
	gceDomainSufix      = "googleapis.com/compute/v1/projects/"
	gcePrefix           = gceURLSchema + "://content." + gceDomainSufix
	instanceURLTemplate = gcePrefix + "%s/zones/%s/instances/%s"
	migURLTemplate      = gcePrefix + "%s/zones/%s/instanceGroups/%s"
)

type credentialFile struct {
	Type      string `json:"type"`
	ProjectID string `json:"project_id"`
}

// getProjectID extracts ProjectId from service account key file
func getProjectID(keyPath string) (string, error) {
	data, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return "", err
	}

	var cred credentialFile
	if err = json.Unmarshal(data, &cred); err != nil {
		return "", err
	}

	if cred.Type != serviceAccountType {
		return "", fmt.Errorf("Invalid service account type: %s", cred.Type)
	}

	return cred.ProjectID, nil
}

func newHTTPClient(ctx context.Context, keyPath string) (*http.Client, error) {
	httpClient, _, err := htransport.NewClient(ctx,
		option.WithScopes(gke.CloudPlatformScope),
		option.WithUserAgent(userAgent),
		option.WithServiceAccountFile(keyPath),
	)
	if err != nil {
		return nil, fmt.Errorf("dialing: %v", err)
	}

	return httpClient, nil
}

func parseGceURL(url, expectedResource string) (project string, zone string, name string, err error) {
	errMsg := fmt.Errorf("Wrong url: expected format https://content.googleapis.com/compute/v1/projects/<project-id>/zones/<zone>/%s/<name>, got %s", expectedResource, url)
	if !strings.Contains(url, gceDomainSufix) {
		return "", "", "", errMsg
	}
	if !strings.HasPrefix(url, gceURLSchema) {
		return "", "", "", errMsg
	}
	splitted := strings.Split(strings.Split(url, gceDomainSufix)[1], "/")
	if len(splitted) != 5 || splitted[1] != "zones" {
		return "", "", "", errMsg
	}
	if splitted[3] != expectedResource {
		return "", "", "", fmt.Errorf("Wrong resource in url: expected %s, got %s", expectedResource, splitted[3])
	}
	project = splitted[0]
	zone = splitted[2]
	name = splitted[4]
	return project, zone, name, nil
}
