package functions

import (
	"errors"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type StructToMapData struct {
	Mode          StructMarshalMode
	Keys          []string
	IgnoreNilFlag bool
}
type StructMarshalMode int

const (
	StructToMapIncludeMode StructMarshalMode = iota
	StructToMapExcludeMode
)

func StructToMap(v any, opts ...StructToMapData) map[string]any {
	// 设置默认值
	var data StructToMapData
	if len(opts) > 0 {
		data = opts[0]
	} else {
		data = StructToMapData{
			Mode:          StructToMapExcludeMode,
			Keys:          make([]string, 0),
			IgnoreNilFlag: false,
		}
	}
	mode := data.Mode
	keys := data.Keys
	ignoreNilFlag := data.IgnoreNilFlag

	resultMap := make(map[string]any)
	vValue := reflect.Indirect(reflect.ValueOf(v)) 

	for i := 0; i < vValue.NumField(); i++ {
		field := vValue.Field(i)
		typeField := vValue.Type().Field(i)
		jsonTag := typeField.Tag.Get("json")
		tagParts := strings.Split(jsonTag, ",")
		jsonKey := tagParts[0]

		if ignoreNilFlag {
			if field.Kind() == reflect.Ptr && field.IsNil() {
				continue
			}
		}

		if jsonTag == "" {
			continue
		}
		if mode == StructToMapIncludeMode {
			if !SliceContainString(keys, jsonKey) {
				continue
			}
		} else {
			if SliceContainString(keys, jsonKey) {
				continue
			}
		}

		if len(tagParts) >= 2 && SliceContainString(tagParts[1:], "omitempty") {
			if !(mode == StructToMapIncludeMode && SliceContainString(keys, jsonKey)) {
				continue
			}
		}

		if field.Type() == reflect.TypeOf(uuid.UUID{}) && field.Interface() == uuid.Nil {
			resultMap[jsonKey] = nil

		} else if field.Type() == reflect.TypeOf(decimal.Decimal{}) {
			resultMap[jsonKey] = field.Interface()
		} else {
			resultMap[jsonKey] = field.Interface()
		}
	}

	return resultMap
}
func SliceContainString(list []string, a string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
func ParseUUID(idString string) (uuid.UUID, error) {
	myUUID, err := uuid.Parse(idString)
	if err != nil {
		return myUUID, errors.New("无法解析UUID")
	}
	return myUUID, nil
}

