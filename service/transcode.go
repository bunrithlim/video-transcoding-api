package service

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nytm/video-transcoding-api/db"
	"github.com/nytm/video-transcoding-api/provider"
)

type newTranscodeRequest struct {
	Source   string
	Profiles []provider.Profile
	Provider string
}

func (s *TranscodingService) newTranscodeJob(r *http.Request) (int, interface{}, error) {
	decoder := json.NewDecoder(r.Body)
	var reqObject newTranscodeRequest
	err := decoder.Decode(&reqObject)
	if err != nil {
		return http.StatusBadRequest, nil, fmt.Errorf("Error while parsing request: %s", err)
	}
	if reqObject.Provider == "" {
		return http.StatusBadRequest, nil, fmt.Errorf("Missing provider from request")
	}
	if reqObject.Source == "" {
		return http.StatusBadRequest, nil, fmt.Errorf("Missing source from request")
	}
	if len(reqObject.Profiles) == 0 {
		return http.StatusBadRequest, nil, fmt.Errorf("Missing profiles from request")
	}
	providerFactory := s.providers[reqObject.Provider]
	if providerFactory == nil {
		return http.StatusBadRequest, nil, fmt.Errorf("Unknown provider found in request: %s", reqObject.Provider)
	}
	providerObj, err := providerFactory(s.config)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if _, ok := err.(provider.InvalidConfigError); ok {
			statusCode = http.StatusBadRequest
		}
		return statusCode, nil, fmt.Errorf("Error initializing provider %s for new job: %v %s", reqObject.Provider, providerObj, err)
	}

	jobStatus, err := providerObj.Transcode(reqObject.Source, reqObject.Profiles)
	if err != nil {
		providerError := fmt.Errorf("Error with provider '%s': %s", reqObject.Provider, err)
		return http.StatusInternalServerError, nil, providerError
	}
	jobStatus.ProviderName = reqObject.Provider

	job := db.Job{ProviderName: jobStatus.ProviderName, ProviderJobID: jobStatus.ProviderJobID}
	err = s.db.SaveJob(&job)
	if err != nil {
		return http.StatusInternalServerError, nil, err
	}
	return 200, map[string]string{"jobId": job.ID}, nil
}

func (s *TranscodingService) getTranscodeJob(r *http.Request) (int, interface{}, error) {
	jobID := mux.Vars(r)["jobId"]
	job, err := s.db.GetJob(jobID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err == db.ErrJobNotFound {
			statusCode = http.StatusNotFound
		}
		return statusCode, nil, fmt.Errorf("Error retrieving job with id '%s': %s", jobID, err)
	}
	providerFactory := s.providers[job.ProviderName]
	if providerFactory == nil {
		return http.StatusInternalServerError, nil, fmt.Errorf("Unknown provider '%s' for job id '%s'", job.ProviderName, jobID)
	}
	providerObj, err := providerFactory(s.config)
	if err != nil {
		return http.StatusInternalServerError, nil, fmt.Errorf("Error initializing provider '%s' on job id '%s': %s %s", job.ProviderName, jobID, providerObj, err)
	}
	jobStatus, err := providerObj.JobStatus(job.ProviderJobID)
	if err != nil {
		providerError := fmt.Errorf("Error with provider '%s' when trying to retrieve job id '%s': %s", job.ProviderName, jobID, err)
		statusCode := http.StatusInternalServerError
		if _, ok := err.(provider.JobNotFoundError); ok {
			statusCode = http.StatusNotFound
		}
		return statusCode, nil, providerError
	}
	jobStatus.ProviderName = job.ProviderName
	return 200, jobStatus, nil
}
