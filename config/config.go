package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-ini/ini"
)

/******************************************************************************/
type Validator func(string) error

type Parameter struct {
	Name       string
	Default    string
	Validators []Validator
}

type Option struct {
	value string
}

type Section struct {
	Options map[string]*Option
}

type Config struct {
	metadata map[string][]Parameter
	Sections map[string]*Section
}

/******************************************************************************/
func OptionsValidatorFactory(options []string) Validator {
	return func(input string) error {
		for _, option := range options {
			if input == option {
				return nil
			}
		}
		return fmt.Errorf("Value (%v) is not one of allowed options: %v", input, options)
	}
}

func BoolValidatorFactory() Validator {
	return func(input string) error {
		_, err := strconv.ParseBool(input)
		return err
	}
}

func IntValidatorFactory() Validator {
	return func(input string) error {
		_, err := strconv.Atoi(input)
		return err
	}
}

func MultiIntValidatorFactory(separator string) Validator {
	return func(input string) error {
		for _, item := range strings.Split(input, separator) {
			_, err := strconv.Atoi(item)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

/******************************************************************************/

func GetAgentConfigMetadata() map[string][]Parameter {
	elements := map[string][]Parameter{
		"default": []Parameter{
			Parameter{"log_file", "/var/log/collectd-sensubility.log", []Validator{}},
			Parameter{"log_level", "INFO", []Validator{OptionsValidatorFactory([]string{"DEBUG", "INFO", "WARNING", "ERROR"})}},
			Parameter{"allow_exec", "true", []Validator{BoolValidatorFactory()}},
		},
		"sensu": []Parameter{
			Parameter{"host", "localhost", []Validator{}},
			Parameter{"port", "5672", []Validator{IntValidatorFactory()}},
			Parameter{"user", "sensu", []Validator{}},
			Parameter{"password", "sensu", []Validator{}},
			Parameter{"vhost", "/sensu", []Validator{}},
			Parameter{"subscriptions", "all,default", []Validator{}},
		},
		"amqp1": []Parameter{
			Parameter{"host", "localhost", []Validator{}},
			Parameter{"port", "5666", []Validator{IntValidatorFactory()}},
			Parameter{"user", "guest", []Validator{}},
			Parameter{"password", "guest", []Validator{}},
		},
	}
	return elements
}

/********** Value methods ***********/
func (opt Option) GetString() string {
	return opt.value
}

func (opt Option) GetStrings(separator string) []string {
	return strings.Split(opt.value, separator)
}

func (opt Option) GetInt() int {
	output, _ := strconv.Atoi(opt.value)
	return output
}

func (opt Option) GetInts(separator string) []int {
	options := strings.Split(opt.value, separator)
	output := make([]int, len(options), len(options))
	for idx, opt := range options {
		output[idx], _ = strconv.Atoi(opt)
	}
	return output
}

func (opt Option) GetBool() bool {
	output, _ := strconv.ParseBool(opt.value)
	return output
}

/** Config methods and fungtions **/
func validate(value string, validators []Validator) error {
	for _, validator := range validators {
		//fmt.Printf("%v - %v\n", value, validator(value))
		err := validator(value)
		if err != nil {
			return fmt.Errorf("Invalid value: %v", value)
		}
	}
	return nil
}

func NewConfig(metadata map[string][]Parameter) (*Config, error) {
	var conf Config
	conf.metadata = metadata
	// initialize config with default values
	conf.Sections = make(map[string]*Section)
	for sectionName, sectionMetadata := range conf.metadata {
		sect := Section{}
		sect.Options = make(map[string]*Option)
		conf.Sections[sectionName] = &sect
		for _, param := range sectionMetadata {
			if err := validate(param.Default, param.Validators); err != nil {
				return nil, fmt.Errorf("Failed to validate parameter %s. %s", param.Name, err.Error())
			}
			opt := Option{param.Default}
			sect.Options[param.Name] = &opt
		}
	}
	return &conf, nil
}

func (conf *Config) Parse(path string) error {
	data, err := ini.Load(path)
	if err != nil {
		return err
	}
	//TODO: log loaded config file
	for sectionName, sectionMetadata := range conf.metadata {
		if sectionData, err := data.GetSection(sectionName); err == nil {
			for _, param := range sectionMetadata {
				if paramData, err := sectionData.GetKey(param.Name); err == nil {
					if err := validate(paramData.Value(), param.Validators); err != nil {
						return fmt.Errorf("Failed to validate parameter %s. %s", param.Name, err.Error())
					}
					conf.Sections[sectionName].Options[param.Name].value = paramData.Value()
				}
			}
		}
	}
	return nil
}
