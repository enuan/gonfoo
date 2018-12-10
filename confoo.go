package confoo

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"

	"gopkg.in/yaml.v2"
)

const confVar = "CONFOO_CONFIG_FILE"

func errorPanic(format string, a ...interface{}) {
	m := fmt.Sprintf(format, a...)
	m2 := "CONFOO - " + m
	panic(m2)
}

var confData map[interface{}]interface{}

func init() {
	confFile := os.Getenv("CONFOO_CONFIG_FILE")
	if confFile == "" {
		errorPanic(confVar + " is not set")
	}
	data, err := ioutil.ReadFile(confFile)
	if err != nil {
		errorPanic(err.Error())
	}

	confData = make(map[interface{}]interface{})
	err = yaml.Unmarshal(data, &confData)
	if err != nil {
		errorPanic("cannot decode yaml data")
	}
}

func Configure(path string, target interface{}) {
	subConf := getSubConf(path, confData)
	if subConf != nil {
		configPath(path, reflect.ValueOf(target), subConf)
	}
}

func getSubConf(path string, conf interface{}) interface{} {
	subConf := conf
	for _, p := range strings.Split(path, ".") {
		m, ok := subConf.(map[interface{}]interface{})
		if !ok {
			errorPanic("%s: path not found", path)
		}

		subConf, ok = m[p]
		if !ok {
			return nil
		}
	}
	return subConf
}

func normalizeKey(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		parts[i] = strings.Title(p)
	}
	return strings.Join(parts, "")
}

func configStruct(path string, dest reflect.Value, conf interface{}) {
	if conf == nil {
		return
	}
	confMap, ok := conf.(map[interface{}]interface{})
	if !ok {
		errorPanic("%s: expected map not found", path)
	}
	for k, subConf := range confMap {
		kk, ok := k.(string)
		if !ok {
			//FIXME log event if CONFOO_DEBUG is set
			//errorPanic("%s.%v: map key is not a string", path, k)
			continue
		}
		fieldName := normalizeKey(kk)
		fieldVal := dest.FieldByName(fieldName)
		if fieldVal.Kind() == reflect.Invalid {
			//FIXME log event if CONFOO_DEBUG is set
			//errorPanic("%s.%v: field not present in target struct", path, k)
			continue
		}
		configPath(path+"."+kk, dest.FieldByName(fieldName), subConf)
	}
}

func configPath(path string, dest reflect.Value, conf interface{}) {
	destKind := dest.Kind()
	switch destKind {
	case reflect.Ptr:
		configPath(path, dest.Elem(), conf)
	case reflect.Struct:
		configStruct(path, dest, conf)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.String, reflect.Bool, reflect.Map:
		confValue := reflect.ValueOf(conf)
		confKind := confValue.Kind()
		if confKind != destKind {
			dType := dest.Type()
			cType := confValue.Type()
			errorPanic("%s: target type %v != conf type %v", path, dType, cType)
		}
		dest.Set(confValue)
	default:
		errorPanic("%s: conf type %v not handled", path, dest.Type())
	}
}
