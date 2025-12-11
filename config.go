package gadm

import (
	"gadm/isdebug"

	"github.com/spf13/cast"
)

type Config map[string]any

func (c Config) Bool(name string, default_value ...bool) bool {
	if v, ok := c[name]; ok {
		return cast.ToBool(v)
	}
	return firstOr(default_value)
}

func (c Config) String(name string, default_value ...string) string {
	if v, ok := c[name]; ok {
		return cast.ToString(v)
	}
	return firstOr(default_value)
}

func (c Config) Int(name string, default_value ...int) int {
	if v, ok := c[name]; ok {
		return cast.ToInt(v)
	}
	return firstOr(default_value)
}

func (c Config) Get(name string) any {
	return c[name]
}

func (c Config) Put(name string, v any) {
	c[name] = v
}

// prefix.name => value
// return name => value
// func (c *Config) GetSection(section string) map[string]any {
// prefix := section + "."
// res := map[string]string{}

// for _, k := range c.dict.Keys() {
// 	if strings.HasPrefix(k, prefix) {
// 		key_without_prefix := k[len(prefix):]
// 		res[key_without_prefix] = c.dict.Get(k)
// 	}
// }
// return nil
// }

// config.Bool("debug.verbose", true)
// net.Dial(config.String("xx.address", "10.0.0.1:3389"))
var config = Config(map[string]any{
	"debug": isdebug.On,
	"theme": "cyborg",
})
