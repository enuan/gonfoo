package confoo

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

const confVar = "CONFOO_CONFIG or CONFOO_CONFIG_FILE"

var confData map[interface{}]interface{}
var confId string
var confWithIdRegex *regexp.Regexp

func init() {
	confFile, confReceived := os.LookupEnv("CONFOO_CONFIG")
	if !confReceived {
		confFile, confReceived = os.LookupEnv("CONFOO_CONFIG_FILE")

		if !confReceived {
			log.Panicf("CONFOO: %s is not set", confVar)
		}
	}

	confId, _ = os.LookupEnv("CONFOO_ID")

	data, err := os.ReadFile(confFile)
	if err != nil {
		log.Panicf("CONFOO: cannot read file %s: %s", confFile, err)
	}

	confData = make(map[interface{}]interface{})
	err = yaml.UnmarshalStrict(data, &confData)
	if err != nil {
		log.Panicf("CONFOO: cannot decode yaml data: %s", err)
	}

	confWithIdRegex = regexp.MustCompile(`(.*)\[(.*)\]$`)
}

// Configure loads the value of the path of the yml of the CONFOO_CONFIG_FILE into target
func Configure(path string, target interface{}) {
	subConf := getSubConf(path, confData)
	if subConf != nil {
		configPath(path, reflect.ValueOf(target), subConf)
	}
}

// ConfigureFromFile reads ymlFile and loads the value of the path into target
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
			log.Panicf("CONFOO: path not found: %s", path)
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
			log.Panicf("CONFOO: Error while retrieving the hostname: %s", error)
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
		log.Panicf("CONFOO: expected map not found: %s", path)
	}
	for k, subConf := range confMap {
		kk, ok := k.(string)
		if !ok {
			log.Printf("CONFOO: map key is not a string: %s.%v", path, k)
			continue
		}
		fieldName := normalizeKey(kk)
		fieldVal := dest.FieldByName(fieldName)
		if fieldVal.Kind() == reflect.Invalid {
			log.Printf("CONFOO: field not present in target struct: %s.%v", path, k)
			continue
		}
		configPath(path+"."+kk, dest.FieldByName(fieldName), subConf)
	}
}

func configPath(path string, dest reflect.Value, conf interface{}) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("CONFOO: panic while handling path %s: %v", path, r)
			log.Printf("CONFOO: stack: %s", string(debug.Stack()))
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
			log.Panicf("CONFOO: target type %v != conf type %v", dType, cType)
		}
		dest.Set(confValue.Convert(dest.Type()))
	case reflect.String:
		conf = replaceValue(conf.(string))
		confValue := reflect.ValueOf(conf)
		confKind := confValue.Kind()
		if confKind != destKind {
			dType := dest.Type()
			cType := confValue.Type()
			log.Panicf("CONFOO: target type %v != conf type %v", dType, cType)
		}
		dest.Set(confValue.Convert(dest.Type()))
	case reflect.Slice:
		dest.Set(dest.Slice(0, 0))
		if conf == nil {
			return
		}
		for i, el := range conf.([]interface{}) {
			idx := strconv.Itoa(i)
			elVal := reflect.New(dest.Type().Elem())
			configPath(path+"."+idx, elVal, el)
			dest.Set(reflect.Append(dest, elVal.Elem()))
		}
	case reflect.Map:
		dest.Set(reflect.MakeMap(dest.Type()))
		if conf == nil {
			return
		}
		for k, el := range conf.(map[interface{}]interface{}) {
			kk := k.(string)
			kk = replaceKey(kk)
			elVal := reflect.New(dest.Type().Elem())
			configPath(path+"."+kk, elVal, el)
			convertedConfValue := reflect.ValueOf(kk).Convert(dest.Type().Key())
			dest.SetMapIndex(convertedConfValue, elVal.Elem())
		}
	default:
		log.Panicf("CONFOO: conf type %v not handled: %s", dest.Type(), path)
	}
}
