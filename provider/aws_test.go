package provider

import (
	"errors"
	"reflect"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/elastictranscoder"
	"github.com/nytm/video-transcoding-api/config"
)

func TestElasticTranscoderProvider(t *testing.T) {
	cfg := config.Config{
		ElasticTranscoder: &config.ElasticTranscoder{
			AccessKeyID:     "AKIANOTREALLY",
			SecretAccessKey: "really-secret",
			PipelineID:      "mypipeline",
			Region:          "sa-east-1",
		},
	}
	provider, err := ElasticTranscoderProvider(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	elasticProvider := provider.(*awsProvider)
	if !reflect.DeepEqual(*elasticProvider.config, *cfg.ElasticTranscoder) {
		t.Errorf("ElasticTranscoderProvider: did not store the proper config. Want %#v. Got %#v.", cfg.ElasticTranscoder, elasticProvider.config)
	}
	expectedCreds := credentials.Value{AccessKeyID: "AKIANOTREALLY", SecretAccessKey: "really-secret"}
	creds, err := elasticProvider.c.(*elastictranscoder.ElasticTranscoder).Config.Credentials.Get()
	if err != nil {
		t.Fatal(err)
	}

	// provider is not relevant
	creds.ProviderName = expectedCreds.ProviderName
	if !reflect.DeepEqual(creds, expectedCreds) {
		t.Errorf("ElasticTranscoderProvider: wrogn credentials. Want %#v. Got %#v.", expectedCreds, creds)
	}

	region := *elasticProvider.c.(*elastictranscoder.ElasticTranscoder).Config.Region
	if region != cfg.ElasticTranscoder.Region {
		t.Errorf("ElasticTranscoderProvider: wrong region. Want %q. Got %q.", cfg.ElasticTranscoder.Region, region)
	}
}

func TestElasticTranscoderProviderDefaultRegion(t *testing.T) {
	cfg := config.Config{
		ElasticTranscoder: &config.ElasticTranscoder{
			AccessKeyID:     "AKIANOTREALLY",
			SecretAccessKey: "really-secret",
			PipelineID:      "mypipeline",
		},
	}
	provider, err := ElasticTranscoderProvider(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	elasticProvider := provider.(*awsProvider)
	if !reflect.DeepEqual(*elasticProvider.config, *cfg.ElasticTranscoder) {
		t.Errorf("ElasticTranscoderProvider: did not store the proper config. Want %#v. Got %#v.", cfg.ElasticTranscoder, elasticProvider.config)
	}
	expectedCreds := credentials.Value{AccessKeyID: "AKIANOTREALLY", SecretAccessKey: "really-secret"}
	creds, err := elasticProvider.c.(*elastictranscoder.ElasticTranscoder).Config.Credentials.Get()
	if err != nil {
		t.Fatal(err)
	}

	// provider is not relevant
	creds.ProviderName = expectedCreds.ProviderName
	if !reflect.DeepEqual(creds, expectedCreds) {
		t.Errorf("ElasticTranscoderProvider: wrogn credentials. Want %#v. Got %#v.", expectedCreds, creds)
	}

	region := *elasticProvider.c.(*elastictranscoder.ElasticTranscoder).Config.Region
	if region != "us-east-1" {
		t.Errorf("ElasticTranscoderProvider: wrong region. Want %q. Got %q.", "us-east-1", region)
	}
}

func TestElasticTranscoderProviderValidation(t *testing.T) {
	var tests = []struct {
		accessKeyID     string
		secretAccessKey string
		pipelineID      string
	}{
		{"", "", ""},
		{"AKIANOTREALLY", "", ""},
		{"", "very-secret", ""},
		{"", "", "superpipeline"},
		{"AKIANOTREALLY", "very-secret", ""},
	}
	for _, test := range tests {
		cfg := config.Config{
			ElasticTranscoder: &config.ElasticTranscoder{
				AccessKeyID:     test.accessKeyID,
				SecretAccessKey: test.secretAccessKey,
				PipelineID:      test.pipelineID,
			},
		}
		provider, err := ElasticTranscoderProvider(&cfg)
		if provider != nil {
			t.Errorf("Got unexpected non-nil provider: %#v", provider)
		}
		if err != errAWSInvalidConfig {
			t.Errorf("Wrong error returned. Want errAWSInvalidConfig. Got %#v", err)
		}
	}
}

func TestAWSTranscode(t *testing.T) {
	fakeTranscoder := newFakeElasticTranscoder()
	provider := &awsProvider{
		c: fakeTranscoder,
		config: &config.ElasticTranscoder{
			AccessKeyID:     "AKIA",
			SecretAccessKey: "secret",
			Region:          "sa-east-1",
			PipelineID:      "mypipeline",
		},
	}
	source := "dir/file.mp4"
	jobStatus, err := provider.TranscodeWithPresets(source, []string{"93239832-0001", "93239832-0002"})
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := regexp.MatchString(`^job-[a-f0-9]{8}$`, jobStatus.ProviderJobID); !m {
		t.Errorf("Elastic Transcoder: invalid id returned - %q", jobStatus.ProviderJobID)
	}
	if jobStatus.Status != StatusQueued {
		t.Errorf("Elastic Transcoder: wrong status returned. Want queued. Got %v", jobStatus.Status)
	}

	if len(fakeTranscoder.jobs) != 1 {
		t.Fatal("Did not send any job request to the server.")
	}
	jobInput := fakeTranscoder.jobs[jobStatus.ProviderJobID]

	expectedJobInput := elastictranscoder.CreateJobInput{
		PipelineId: aws.String("mypipeline"),
		Input:      &elastictranscoder.JobInput{Key: aws.String(source)},
		Outputs: []*elastictranscoder.CreateJobOutput{
			{PresetId: aws.String("93239832-0001"), Key: aws.String("dir/93239832-0001/file.mp4")},
			{PresetId: aws.String("93239832-0002"), Key: aws.String("dir/93239832-0002/file.mp4")},
		},
	}
	if !reflect.DeepEqual(*jobInput, expectedJobInput) {
		t.Errorf("Elastic Transcoder: wrong input. Want %#v. Got %#v.", expectedJobInput, *jobInput)
	}
}

func TestAWSTranscodeAWSFailure(t *testing.T) {
	prepErr := errors.New("something went wrong")
	fakeTranscoder := newFakeElasticTranscoder()
	fakeTranscoder.prepareFailure("CreateJob", prepErr)
	provider := &awsProvider{
		c: fakeTranscoder,
		config: &config.ElasticTranscoder{
			AccessKeyID:     "AKIA",
			SecretAccessKey: "secret",
			Region:          "sa-east-1",
			PipelineID:      "mypipeline",
		},
	}
	source := "dir/file.mp4"
	jobStatus, err := provider.TranscodeWithPresets(source, []string{"93239832-0001", "93239832-0002"})
	if jobStatus != nil {
		t.Errorf("Got unexpected non-nil status: %#v", jobStatus)
	}
	if err != prepErr {
		t.Errorf("Got wrong error. Want %q. Got %q", prepErr.Error(), err.Error())
	}
}

func TestAWSJobStatus(t *testing.T) {
	fakeTranscoder := newFakeElasticTranscoder()
	provider := &awsProvider{
		c: fakeTranscoder,
		config: &config.ElasticTranscoder{
			AccessKeyID:     "AKIA",
			SecretAccessKey: "secret",
			Region:          "sa-east-1",
			PipelineID:      "mypipeline",
		},
	}
	jobStatus, err := provider.TranscodeWithPresets("dir/file.mp4", []string{"93239832-0001", "93239832-0002"})
	if err != nil {
		t.Fatal(err)
	}
	id := jobStatus.ProviderJobID
	jobStatus, err = provider.JobStatus(id)
	if err != nil {
		t.Fatal(err)
	}
	expectedJobStatus := JobStatus{
		ProviderJobID: id,
		Status:        StatusFinished,
	}
	if !reflect.DeepEqual(*jobStatus, expectedJobStatus) {
		t.Errorf("Wrong JobStatus. Want %#v. Got %#v.", expectedJobStatus, *jobStatus)
	}
}

func TestAWSJobStatusNotFound(t *testing.T) {
	fakeTranscoder := newFakeElasticTranscoder()
	provider := &awsProvider{
		c: fakeTranscoder,
		config: &config.ElasticTranscoder{
			AccessKeyID:     "AKIA",
			SecretAccessKey: "secret",
			Region:          "sa-east-1",
			PipelineID:      "mypipeline",
		},
	}
	jobStatus, err := provider.JobStatus("idk")
	if err == nil {
		t.Fatal("Got unexpected <nil> error")
	}
	expectedErrMsg := "job not found"
	if err.Error() != expectedErrMsg {
		t.Errorf("Got wrong error message. Want %q. Got %q", expectedErrMsg, err.Error())
	}
	if jobStatus != nil {
		t.Errorf("Got unexpected non-nil JobStatus: %#v", jobStatus)
	}
}

func TestAWSJobStatusInternalError(t *testing.T) {
	prepErr := errors.New("failed to get job status")
	fakeTranscoder := newFakeElasticTranscoder()
	fakeTranscoder.prepareFailure("ReadJob", prepErr)
	provider := &awsProvider{
		c: fakeTranscoder,
		config: &config.ElasticTranscoder{
			AccessKeyID:     "AKIA",
			SecretAccessKey: "secret",
			Region:          "sa-east-1",
			PipelineID:      "mypipeline",
		},
	}
	jobStatus, err := provider.JobStatus("idk")
	if jobStatus != nil {
		t.Errorf("Got unexpected non-nil JobStatus: %#v", jobStatus)
	}
	if err != prepErr {
		t.Errorf("Got wrong error. Want %q. Got %q", prepErr.Error(), err.Error())
	}
}

func TestAWSStatusMap(t *testing.T) {
	var tests = []struct {
		input  string
		output status
	}{
		{"Submitted", StatusQueued},
		{"Progressing", StatusStarted},
		{"Canceled", StatusCanceled},
		{"Error", StatusFailed},
		{"Complete", StatusFinished},
		{"unknown", StatusFailed},
	}
	var prov awsProvider
	for _, test := range tests {
		result := prov.statusMap(test.input)
		if result != test.output {
			t.Errorf("statusMap(%q): wrong result. Want %q. Got %q", test.input, test.output, result)
		}
	}
}