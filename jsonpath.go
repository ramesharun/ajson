package ajson

import (
	"io"
	"math"
	"strconv"
	"strings"
)

// JSONPath returns slice of founded elements in current JSON data, by it's JSONPath.
//
// JSONPath expressions:
//
//	$	the root object/element
//	@	the current object/element
//	. or []	child operator
//	..	recursive descent. JSONPath borrows this syntax from E4X.
//	*	wildcard. All objects/elements regardless their names.
//	[]	subscript operator. XPath uses it to iterate over element collections and for predicates. In Javascript and JSON it is the native array operator.
//	[,]	Union operator in XPath results in a combination of node sets. JSONPath allows alternate names or array indices as a set.
//	[start:end:step]	array slice operator borrowed from ES4.
//	?()	applies a filter (script) expression.
//	()	script expression, using the underlying script engine.
func JSONPath(data []byte, path string) (result []*Node, err error) {
	commands, err := ParseJSONPath(path)
	if err != nil {
		return nil, err
	}
	root, err := Unmarshal(data)
	if err != nil {
		return nil, err
	}
	result = make([]*Node, 0)

	var (
		temporary      []*Node
		keys           []string
		from, to, step int
	)
	for i, cmd := range commands {
		switch {
		case cmd == "$": // root element
			if i == 0 {
				result = append(result, root)
			}
		case cmd == "..": // recursive descent
			temporary = make([]*Node, 0)
			for _, element := range result {
				temporary = append(temporary, recursiveChildren(element)...)
			}
			result = append(result, temporary...)
		case cmd == "*": // wildcard
			temporary = make([]*Node, 0)
			for _, element := range result {
				temporary = append(temporary, element.inheritors()...)
			}
			result = temporary
		case strings.Contains(cmd, ":"): // array slice operator
			keys = strings.Split(cmd, ":")
			if len(keys) > 3 {
				return nil, errorRequest()
			}
			if keys[0] == "" {
				from = 0
			} else {
				from, err = strconv.Atoi(keys[0])
				if err != nil {
					return nil, errorRequest()
				}
			}
			if keys[1] == "" {
				to = math.MaxInt64
			} else {
				to, err = strconv.Atoi(keys[1])
				if err != nil {
					return nil, errorRequest()
				}
			}
			step = 1
			if len(keys) == 3 {
				if keys[2] != "" {
					step, err = strconv.Atoi(keys[2])
					if err != nil {
						return nil, errorRequest()
					}
				}
			}

			temporary = make([]*Node, 0)
			for _, element := range result {
				if element.IsArray() {
					for i := from; i < to; i += step {
						value, ok := element.children[strconv.Itoa(i)]
						if ok {
							temporary = append(temporary, value)
						} else {
							break
						}
					}
				}
			}
			result = temporary
		case strings.HasPrefix(cmd, "?("): // applies a filter (script) expression
			//$..[?(@.price == 19.95 && @.color == 'red')].color

		//case strings.HasPrefix(cmd, "("): // script expression, using the underlying script engine
		default: // try to get by key & Union
			keys = strings.Split(cmd, ",")
			temporary = make([]*Node, 0)
			for _, key := range keys {
				for _, element := range result {
					if element.isContainer() {
						value, ok := element.children[key]
						if ok {
							temporary = append(temporary, value)
						}
					}
				}
			}
			result = temporary
		}
	}
	return
}

//Paths returns calculated paths of underlying nodes
func Paths(array []*Node) []string {
	result := make([]string, 0, len(array))
	for _, element := range array {
		result = append(result, element.Path())
	}
	return result
}

func recursiveChildren(node *Node) (result []*Node) {
	if node.isContainer() {
		for _, element := range node.inheritors() {
			if element.isContainer() {
				result = append(result, element)
			}
		}
	}
	temp := make([]*Node, 0, len(result))
	temp = append(temp, result...)
	for _, element := range result {
		temp = append(temp, recursiveChildren(element)...)
	}
	return temp
}

//ParseJSONPath will parse current path and return all commands tobe run.
func ParseJSONPath(path string) (result []string, err error) {
	buf := newBuffer([]byte(path))
	result = make([]string, 0)
	var (
		b           byte
		start, stop int
		childEnd    = map[byte]bool{dot: true, bracketL: true}
	)
	for {
		b, err = buf.current()
		if err != nil {
			break
		}
		switch true {
		case b == dollar:
			result = append(result, "$")
		case b == dot:
			start = buf.index
			b, err = buf.next()
			if err == io.EOF {
				err = nil
				break
			}
			if err != nil {
				break
			}
			if b == dot {
				result = append(result, "..")
				buf.index--
				break
			}
			err = buf.skipAny(childEnd)
			stop = buf.index
			if err == io.EOF {
				err = nil
				stop = buf.length
			} else {
				buf.index--
			}
			if err != nil {
				break
			}
			if start+1 < stop {
				result = append(result, string(buf.data[start+1:stop]))
			}
		case b == bracketL:
			b, err = buf.next()
			if err != nil {
				return nil, buf.errorEOF()
			}
			start = buf.index
			if b == quote {
				start++
				err = buf.string(quote)
				if err != nil {
					return nil, buf.errorEOF()
				}
				stop = buf.index
				b, err = buf.next()
				if err != nil {
					return nil, buf.errorEOF()
				}
				if b != bracketR {
					return nil, buf.errorSymbol()
				}
			} else {
				err = buf.skip(bracketR)
				stop = buf.index
				if err != nil {
					return nil, buf.errorEOF()
				}
			}
			result = append(result, string(buf.data[start:stop]))
		default:
			return nil, buf.errorSymbol()
		}
		err = buf.step()
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			break
		}
	}
	return
}
