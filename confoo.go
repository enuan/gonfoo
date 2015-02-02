package confetto

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v2"
)

var (
	regs       map[string]interface{}
	configured bool
	errors     []string
)

func saveError(format string, a ...interface{}) {
	m := fmt.Sprintf(format, a...)
	errors = append(errors, m)
}

func Register(path string, dest interface{}) {
	// FIXME in realtà basterebbe verificare che sia settabile
	if reflect.TypeOf(dest).Kind() != reflect.Ptr {
		// FIXME
		panic("destination is not a pointer type")
	}
	if regs == nil {
		regs = make(map[string]interface{})
	}
	if _, ok := regs[path]; ok {
		saveError("%s: path already registerd")
	}
	regs[path] = dest
}

func Configure(yamlData []byte) {
	if configured {
		panic("already configured")
	}
	configured = true
	conf := make(map[interface{}]interface{})
	err := yaml.Unmarshal(yamlData, &conf)
	if err != nil {
		panic("cannot decode yaml data")
	}

	for path, dest := range regs {
		subConf := getSubConf(path, conf)
		if subConf != nil {
			configPath(path, reflect.ValueOf(dest), subConf)
		}
	}

	//FIXME
	fmt.Printf("%d error(s)\n", len(errors))
	for _, m := range errors {
		fmt.Println(m)
	}
}

func getSubConf(path string, conf interface{}) interface{} {
	subConf := conf
	for _, p := range strings.Split(path, ".") {
		m, ok := subConf.(map[interface{}]interface{})
		if !ok {
			saveError("%s: path not found", path)
			return nil
		}

		subConf, ok = m[p]
		if !ok {
			return nil
		}
	}
	return subConf
}

func configStruct(path string, dest reflect.Value, conf interface{}) {
	if conf == nil {
		return
	}
	confMap, ok := conf.(map[interface{}]interface{})
	if !ok {
		saveError("%s: expected map not found", path)
		return
	}
	for k, subConf := range confMap {
		kk, ok := k.(string)
		if !ok {
			saveError("%s.%v: map key is not a string", path, k)
			return
		}
		// TODO Title is arbitrary
		fieldName := strings.Title(kk)
		fieldVal := dest.FieldByName(fieldName)
		if fieldVal.Kind() == reflect.Invalid {
			saveError("%s.%v: field not present in target struct", path, k)
			return
		}
		configPath(path+"."+kk, dest.FieldByName(fieldName), subConf)
	}
}

func configPath(path string, dest reflect.Value, conf interface{}) {
	fmt.Printf("path=%s %v\n", path, dest)
	destKind := dest.Kind()
	switch destKind {
	case reflect.Ptr:
		configPath(path, dest.Elem(), conf)
	case reflect.Struct:
		configStruct(path, dest, conf)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.String, reflect.Bool:
		confValue := reflect.ValueOf(conf)
		confKind := confValue.Kind()
		if confKind != destKind {
			dType := dest.Type()
			cType := confValue.Type()
			saveError("%s: target type %v != conf type %v", dType, cType)
			return
		}
		dest.Set(confValue)
	default:
		saveError("%s: conf type %v not handled", dest.Type())
		return
	}
}
