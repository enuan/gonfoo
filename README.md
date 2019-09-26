# confoo.go
Yaml based configuration management library for go

Example
-------

Setup the configuration file:

```
$ export CONFOO_CONFIG_FILE $HOME/confoo.config
$ cat $CONFOO_CONFIG_FILE
test: 
    foo: 
        host: www.paolo.com
        port: 80
        params: 
            p1: mega
            flag: true
		items:
			- field_one: value
			  field_two: value
		options:
			username: john_connor
			password: qwerty
```

Sample executable:

```Go
package main

import (
	"fmt"

	"github.com/enuan/gonfoo"
)

type Options struct {
	Username string
	Password string
}

var config struct {
	Host   string
	Port   int
	Params struct {
		P1   string
		P2   string
		Flag bool
	}
	// slices of any structs
	Items []struct {
		FieldOne string
		FieldTwo string
	}
	// Options pointer fields
	Options *Options
}

func init() {
	config.Host = "foo.bar.com"
	config.Port = 123
	config.Params.P1 = "bar"
	config.Params.P2 = "foo"

	confoo.Configure("test.foo", &config)
}

func main() {
	fmt.Println("config", config)
}
```
