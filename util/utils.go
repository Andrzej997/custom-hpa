package util

import (
	"reflect"
	"time"
)

func SetInterval(executableFunc func(), duration time.Duration, async bool) chan bool {
	interval := duration
	ticker := time.NewTicker(interval)
	clearInterval := make(chan bool)

	go func() {
		for {
			select {
			case <-ticker.C:
				if async {
					go executableFunc()
				} else {
					executableFunc()
				}
			case <-clearInterval:
				ticker.Stop()
				return
			}
		}
	}()
	return clearInterval
}

func SetTimeout(executableFunc func(), duration time.Duration) {
	timeout := duration
	time.AfterFunc(timeout, executableFunc)
}

func flattenDeepInternal(args []interface{}, v reflect.Value) []interface{} {

	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}

	if v.Kind() == reflect.Array || v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			args = flattenDeepInternal(args, v.Index(i))
		}
	} else {
		args = append(args, v.Interface())
	}

	return args
}

func FlattenDeep(args ...interface{}) []interface{} {
	return flattenDeepInternal(nil, reflect.ValueOf(args))
}
