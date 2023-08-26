package types

type DestinationType string

const (
	Aws DestinationType = "aws"
)

type DestinationFile struct {
	Destinations []Destination `yaml:"destinations"`
}

type Destination struct {
	Name   string          `yaml:"name"`
	Type   DestinationType `yaml:"type"`
	Bucket string          `yaml:"bucket"`
	Region string          `yaml:"region"`
}
