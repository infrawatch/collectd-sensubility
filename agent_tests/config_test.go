package agent_tests

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/paramite/collectd-sensubility/agent"
	"github.com/stretchr/testify/assert"
)

var CONFIG_CONTENT = `
[default]
log_file=/var/tmp/test.log
allow_exec=false

[sensu]
port=666

[invalid]
IntValidator=whoops
MultiIntValidator=1,2,whoops,4
BoolValidator=no-way
OptionsValidator=foo
`

type validatorTest struct {
	parameter string
	validator agent.Validator
	defValue  string
}

func TestConfigValues(t *testing.T) {
	// create temporary config file
	file, err := ioutil.TempFile(".", "agent_config_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())
	// save test content
	file.WriteString(CONFIG_CONTENT)
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	// test parsing
	metadata := agent.GetAgentConfigMetadata()
	conf, err := agent.NewConfig(metadata)
	if err != nil {
		t.Fatal(err)
	}
	err = conf.Parse(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	// test parsed sections
	sections := []string{}
	for key, _ := range conf.Sections {
		sections = append(sections, key)
	}
	assert.ElementsMatch(t, []string{"default", "sensu", "amqp1"}, sections)
	// test default values
	assert.Equal(t, []string{"all", "default"}, conf.Sections["sensu"].Options["subscriptions"].GetStrings(","))
	// test parsed overrided values
	assert.Equal(t, "/var/tmp/test.log", conf.Sections["default"].Options["log_file"].GetString(), "Did not parse correctly")
	assert.Equal(t, false, conf.Sections["default"].Options["allow_exec"].GetBool(), "Did not parse correctly")
	assert.Equal(t, 666, conf.Sections["sensu"].Options["port"].GetInt(), "Did not parse correctly")
}

func TestValidators(t *testing.T) {
	// create temporary config file
	file, err := ioutil.TempFile(".", "agent_config_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())
	// save test content
	file.WriteString(CONFIG_CONTENT)
	err = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	// test failing parsing (each validator separately)
	tests := []validatorTest{
		validatorTest{"IntValidator", agent.IntValidatorFactory(), "3"},
		validatorTest{"MultiIntValidator", agent.MultiIntValidatorFactory(","), "1,2"},
		validatorTest{"BoolValidator", agent.BoolValidatorFactory(), "true"},
		validatorTest{"OptionsValidator", agent.OptionsValidatorFactory([]string{"bar", "baz"}), "bar"},
	}
	for _, test := range tests {
		metadata := map[string][]agent.Parameter{
			"invalid": []agent.Parameter{
				agent.Parameter{test.parameter, test.defValue, []agent.Validator{test.validator}},
			},
		}
		conf, err := agent.NewConfig(metadata)
		err = conf.Parse(file.Name())
		if err == nil {
			t.Errorf("Failed to report validation error with %s.", test.parameter)
		}
	}
	// test failing constructor (validation of default values)
	metadata := map[string][]agent.Parameter{
		"invalid": []agent.Parameter{
			agent.Parameter{"default_test", "default", []agent.Validator{agent.IntValidatorFactory()}},
		},
	}
	_, err = agent.NewConfig(metadata)
	if err == nil {
		t.Errorf("Failed to report validation error in constructor.")
	}
}
