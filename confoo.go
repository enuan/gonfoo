package confoo

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

const confVar = "CONFOO_CONFIG or CONFOO_CONFIG_FILE"

func errorPanic(format string, a ...interface{}) {
	m := fmt.Sprintf(format, a...)
	m2 := "CONFOO - " + m
	panic(m2)
}

var confData map[interface{}]interface{}
var confId string
var confWithIdRegex *regexp.Regexp

func init() {
	confFile, confReceived := os.LookupEnv("CONFOO_CONFIG")
	if !confReceived {
		confFile, confReceived = os.LookupEnv("CONFOO_CONFIG_FILE")

		if !confReceived {
			errorPanic(confVar + " is not set")
		}
	}

	confId, _ = os.LookupEnv("CONFOO_ID")

	data, err := ioutil.ReadFile(confFile)
	if err != nil {
		errorPanic(err.Error())
	}

	confData = make(map[interface{}]interface{})
	err = yaml.Unmarshal(data, &confData)
	if err != nil {
		errorPanic("cannot decode yaml data")
	}

	confWithIdRegex = regexp.MustCompile(`(.*)\[(.*)\]$`)
}

//Configure loads the value of the path of the yml of the CONFOO_CONFIG_FILE into target
func Configure(path string, target interface{}) {
	subConf := getSubConf(path, confData)
	if subConf != nil {
		configPath(path, reflect.ValueOf(target), subConf)
	}
}

//ConfigureFromFile reads ymlFile and loads the value of the path into target
func ConfigureFromFile(ymlFile, path string, target interface{}) error {

	data, err := ioutil.ReadFile(ymlFile)
	if err != nil {
		return err
	}

	confData := make(map[interface{}]interface{})
	err = yaml.Unmarshal(data, &confData)
	if err != nil {
		return err
	}

	subConf := getSubConf(path, confData)
	if subConf != nil {
		configPath(path, reflect.ValueOf(target), subConf)
	}
	return nil
}

func coerceIdentity(conf interface{}) {
	confMap, ok := conf.(map[interface{}]interface{})
	if ok {
		for key, value := range confMap {
			if _, ok := key.(string); !ok {
				continue
			}

			coerceIdentity(value)
			match := confWithIdRegex.FindSubmatch([]byte(key.(string)))
			if match == nil {
				continue
			}
			delete(confMap, key)
			realKey := string(match[1])
			identity := string(match[2])
			if identity == confId {
				confMap[realKey] = value
			}
		}
	}
	confList, ok := conf.([]interface{})
	if ok {
		for _, entry := range confList {
			coerceIdentity(entry)
		}
	}
}

func getSubConf(path string, conf interface{}) interface{} {

	coerceIdentity(conf)
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

func replaceKey(s string) string {
	if strings.Contains(s, "$hostname") {
		hostname, error := os.Hostname()
		if error != nil {
			errorPanic("Error while retrieving the hostname: %s", error)
		}

		return strings.Replace(s, "$hostname", hostname, -1)
	}

	return s
}

func replaceValue(s string) string {
	if strings.Contains(s, "$public_hostname") {
		content, err := ioutil.ReadFile("/etc/public_hostname")
		if err != nil {
			fmt.Println(err)
			return s
		}

		public_hostname := strings.TrimSuffix(string(content), "\n")
		return strings.Replace(s, "$public_hostname", public_hostname, -1)
	}

	return s
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
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "panic while handling path %s: %v\n", path, r)
			fmt.Fprintf(os.Stderr, string(debug.Stack()))
			os.Exit(1)
		}
	}()
	destKind := dest.Kind()
	switch destKind {
	case reflect.Ptr:
		if dest.Type().Elem().Kind() == reflect.Struct && dest.IsNil() {
			dest.Set(reflect.New(dest.Type().Elem()))
		}
		configPath(path, dest.Elem(), conf)
	case reflect.Interface:
		dest.Set(reflect.ValueOf(conf))
	case reflect.Struct:
		configStruct(path, dest, conf)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Float32, reflect.Float64, reflect.Bool:
		confValue := reflect.ValueOf(conf)
		confKind := confValue.Kind()
		if confKind != destKind {
			dType := dest.Type()
			cType := confValue.Type()
			errorPanic("%s: target type %v != conf type %v", path, dType, cType)
		}
		dest.Set(confValue)
	case reflect.String:
		conf = replaceValue(conf.(string))
		confValue := reflect.ValueOf(conf)
		confKind := confValue.Kind()
		if confKind != destKind {
			dType := dest.Type()
			cType := confValue.Type()
			errorPanic("%s: target type %v != conf type %v", path, dType, cType)
		}
		dest.Set(confValue)
	case reflect.Slice:
		dest.Set(dest.Slice(0, 0))
		for i, el := range conf.([]interface{}) {
			idx := strconv.Itoa(i)
			elVal := reflect.New(dest.Type().Elem())
			configPath(path+"."+idx, elVal, el)
			dest.Set(reflect.Append(dest, elVal.Elem()))
		}
	case reflect.Map:
		dest.Set(reflect.MakeMap(dest.Type()))
		for k, el := range conf.(map[interface{}]interface{}) {
			kk := k.(string)
			kk = replaceKey(kk)
			elVal := reflect.New(dest.Type().Elem())
			configPath(path+"."+kk, elVal, el)
			dest.SetMapIndex(reflect.ValueOf(kk), elVal.Elem())
		}
	default:
		errorPanic("%s: conf type %v not handled", path, dest.Type())
	}
}
