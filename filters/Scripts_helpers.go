package filters

import "github.com/grafana/sobek/ast"
import "github.com/grafana/sobek/parser"

var js_network_callees = map[string]bool{
	"fetch":          true,
	"sendBeacon":     true,
	"XMLHttpRequest": true,
	"WebSocket":      true,
	"EventSource":    true,
	"import":         true,
	"axios":          true,
	"ajax":           true,
	"jsonp":          true,
	"getJSON":        true,
	"postJSON":       true,
}

var js_network_methods = map[string]bool{
	"send":    true,
	"open":    true,
	"ajax":    true,
	"request": true,
	"fetch":   true,
	"jsonp":   true,
}

var js_network_constructors = map[string]bool{
	"XMLHttpRequest": true,
	"WebSocket":      true,
	"EventSource":    true,
	"Image":          true,
	"Audio":          true,
	"Worker":         true,
}

type js_spoof struct {
	start int
	end   int
	body  string
}


func js_splice(source []byte, start int, end int, body string) []byte {

	result := make([]byte, 0, len(source)-(end-start)+len(body))
	result = append(result, source[:start]...)
	result = append(result, body...)
	result = append(result, source[end:]...)

	return result

}

func find_js_functions(statement ast.Statement, spoofs *[]js_spoof) {

	switch current := statement.(type) {

	case *ast.FunctionDeclaration:
		handle_js_function(current.Function, spoofs)

	case *ast.ClassDeclaration:
		for _, element := range current.Class.Body {
			if method, ok := element.(*ast.MethodDefinition); ok == true {
				handle_js_function(method.Body, spoofs)
			}
		}

	case *ast.BlockStatement:
		for _, child := range current.List {
			find_js_functions(child, spoofs)
		}

	case *ast.IfStatement:
		find_js_functions(current.Consequent, spoofs)
		if current.Alternate != nil {
			find_js_functions(current.Alternate, spoofs)
		}

	case *ast.ForStatement:
		find_js_functions(current.Body, spoofs)

	case *ast.ForInStatement:
		find_js_functions(current.Body, spoofs)

	case *ast.ForOfStatement:
		find_js_functions(current.Body, spoofs)

	case *ast.WhileStatement:
		find_js_functions(current.Body, spoofs)

	case *ast.DoWhileStatement:
		find_js_functions(current.Body, spoofs)

	case *ast.LabelledStatement:
		find_js_functions(current.Statement, spoofs)

	case *ast.WithStatement:
		find_js_functions(current.Body, spoofs)

	case *ast.SwitchStatement:
		for _, current_case := range current.Body {
			for _, child := range current_case.Consequent {
				find_js_functions(child, spoofs)
			}
		}

	case *ast.TryStatement:
		find_js_functions(current.Body, spoofs)
		if current.Catch != nil && current.Catch.Body != nil {
			find_js_functions(current.Catch.Body, spoofs)
		}
		if current.Finally != nil {
			find_js_functions(current.Finally, spoofs)
		}

	case *ast.ThrowStatement:
		if current.Argument != nil {
			find_js_functions_expr(current.Argument, spoofs)
		}

	case *ast.ReturnStatement:
		if current.Argument != nil {
			find_js_functions_expr(current.Argument, spoofs)
		}

	case *ast.ExpressionStatement:
		find_js_functions_expr(current.Expression, spoofs)

	case *ast.VariableStatement:
		for _, binding := range current.List {
			find_js_functions_binding(binding, spoofs)
		}

	case *ast.LexicalDeclaration:
		for _, binding := range current.List {
			find_js_functions_binding(binding, spoofs)
		}

	}

}

func find_js_functions_expr(expression ast.Expression, spoofs *[]js_spoof) {

	switch current := expression.(type) {

	case *ast.FunctionLiteral:
		handle_js_function(current, spoofs)

	case *ast.ArrowFunctionLiteral:
		handle_js_arrow(current, spoofs)

	case *ast.ClassLiteral:
		for _, element := range current.Body {
			if method, ok := element.(*ast.MethodDefinition); ok == true {
				handle_js_function(method.Body, spoofs)
			}
		}

	case *ast.CallExpression:
		find_js_functions_expr(current.Callee, spoofs)
		for _, argument := range current.ArgumentList {
			find_js_functions_expr(argument, spoofs)
		}

	case *ast.NewExpression:
		find_js_functions_expr(current.Callee, spoofs)
		for _, argument := range current.ArgumentList {
			find_js_functions_expr(argument, spoofs)
		}

	case *ast.DotExpression:
		find_js_functions_expr(current.Left, spoofs)

	case *ast.BracketExpression:
		find_js_functions_expr(current.Left, spoofs)
		find_js_functions_expr(current.Member, spoofs)

	case *ast.AssignExpression:
		find_js_functions_expr(current.Left, spoofs)
		find_js_functions_expr(current.Right, spoofs)

	case *ast.BinaryExpression:
		find_js_functions_expr(current.Left, spoofs)
		find_js_functions_expr(current.Right, spoofs)

	case *ast.UnaryExpression:
		find_js_functions_expr(current.Operand, spoofs)

	case *ast.AwaitExpression:
		find_js_functions_expr(current.Argument, spoofs)

	case *ast.YieldExpression:
		if current.Argument != nil {
			find_js_functions_expr(current.Argument, spoofs)
		}

	case *ast.ConditionalExpression:
		find_js_functions_expr(current.Test, spoofs)
		find_js_functions_expr(current.Consequent, spoofs)
		find_js_functions_expr(current.Alternate, spoofs)

	case *ast.SequenceExpression:
		for _, child := range current.Sequence {
			find_js_functions_expr(child, spoofs)
		}

	case *ast.ArrayLiteral:
		for _, child := range current.Value {
			find_js_functions_expr(child, spoofs)
		}

	case *ast.ObjectLiteral:
		for _, property := range current.Value {
			find_js_functions_property(property, spoofs)
		}

	case *ast.TemplateLiteral:
		for _, child := range current.Expressions {
			find_js_functions_expr(child, spoofs)
		}

	}

}

func find_js_functions_binding(binding *ast.Binding, spoofs *[]js_spoof) {

	if binding != nil && binding.Initializer != nil {
		find_js_functions_expr(binding.Initializer, spoofs)
	}

}

func find_js_functions_property(property ast.Property, spoofs *[]js_spoof) {

	switch current := property.(type) {

	case *ast.PropertyShort:
		if current.Initializer != nil {
			find_js_functions_expr(current.Initializer, spoofs)
		}

	case *ast.PropertyKeyed:
		find_js_functions_expr(current.Key, spoofs)
		find_js_functions_expr(current.Value, spoofs)

	case *ast.SpreadElement:
		find_js_functions_expr(current.Expression, spoofs)

	}

}

func handle_js_function(function *ast.FunctionLiteral, spoofs *[]js_spoof) {

	if function == nil || function.Body == nil {
		return
	}

	if js_block_has_network(function.Body) == true {

		body := "{ " + js_farble_return(function.Body) + " }"

		*spoofs = append(*spoofs, js_spoof{
			start: int(function.Body.Idx0()) - 1,
			end:   int(function.Body.Idx1()) - 1,
			body:  body,
		})

		return

	}

	for _, child := range function.Body.List {
		find_js_functions(child, spoofs)
	}

}

func handle_js_arrow(function *ast.ArrowFunctionLiteral, spoofs *[]js_spoof) {

	if function == nil {
		return
	}

	switch body := function.Body.(type) {

	case *ast.BlockStatement:

		if js_block_has_network(body) == true {

			*spoofs = append(*spoofs, js_spoof{
				start: int(body.Idx0()) - 1,
				end:   int(body.Idx1()) - 1,
				body:  "{ " + js_farble_return(body) + " }",
			})

			return

		}

		for _, child := range body.List {
			find_js_functions(child, spoofs)
		}

	case *ast.ExpressionBody:

		if js_expr_has_network(body.Expression) == true {

			*spoofs = append(*spoofs, js_spoof{
				start: int(body.Expression.Idx0()) - 1,
				end:   int(body.Expression.Idx1()) - 1,
				body:  "true",
			})

		} else {
			find_js_functions_expr(body.Expression, spoofs)
		}

	}

}

func js_block_has_network(block *ast.BlockStatement) bool {

	if block == nil {
		return false
	}

	for _, statement := range block.List {

		if js_stmt_has_network(statement) == true {
			return true
		}

	}

	return false

}

func js_stmt_has_network(statement ast.Statement) bool {

	switch current := statement.(type) {

	case *ast.FunctionDeclaration:
		return false

	case *ast.ExpressionStatement:
		return js_expr_has_network(current.Expression)

	case *ast.ReturnStatement:
		return current.Argument != nil && js_expr_has_network(current.Argument)

	case *ast.VariableStatement:
		for _, binding := range current.List {
			if js_binding_has_network(binding) == true {
				return true
			}
		}
		return false

	case *ast.LexicalDeclaration:
		for _, binding := range current.List {
			if js_binding_has_network(binding) == true {
				return true
			}
		}
		return false

	case *ast.BlockStatement:
		for _, child := range current.List {
			if js_stmt_has_network(child) == true {
				return true
			}
		}
		return false

	case *ast.IfStatement:
		return js_expr_has_network(current.Test) ||
			js_stmt_has_network(current.Consequent) ||
			(current.Alternate != nil && js_stmt_has_network(current.Alternate))

	case *ast.ForStatement:
		return (current.Test != nil && js_expr_has_network(current.Test)) ||
			(current.Update != nil && js_expr_has_network(current.Update)) ||
			js_stmt_has_network(current.Body)

	case *ast.ForInStatement:
		return js_expr_has_network(current.Source) || js_stmt_has_network(current.Body)

	case *ast.ForOfStatement:
		return js_expr_has_network(current.Source) || js_stmt_has_network(current.Body)

	case *ast.WhileStatement:
		return js_expr_has_network(current.Test) || js_stmt_has_network(current.Body)

	case *ast.DoWhileStatement:
		return js_expr_has_network(current.Test) || js_stmt_has_network(current.Body)

	case *ast.SwitchStatement:
		if js_expr_has_network(current.Discriminant) == true {
			return true
		}
		for _, current_case := range current.Body {
			for _, child := range current_case.Consequent {
				if js_stmt_has_network(child) == true {
					return true
				}
			}
		}
		return false

	case *ast.TryStatement:
		return js_stmt_has_network(current.Body) ||
			(current.Catch != nil && current.Catch.Body != nil && js_stmt_has_network(current.Catch.Body)) ||
			(current.Finally != nil && js_stmt_has_network(current.Finally))

	case *ast.ThrowStatement:
		return current.Argument != nil && js_expr_has_network(current.Argument)

	case *ast.LabelledStatement:
		return js_stmt_has_network(current.Statement)

	case *ast.WithStatement:
		return js_expr_has_network(current.Object) || js_stmt_has_network(current.Body)

	}

	return false

}

func js_expr_has_network(expression ast.Expression) bool {

	switch current := expression.(type) {

	case *ast.CallExpression:
		if js_callee_is_network(current.Callee) == true {
			return true
		}
		if js_expr_has_network(current.Callee) == true {
			return true
		}
		for _, argument := range current.ArgumentList {
			if js_expr_has_network(argument) == true {
				return true
			}
		}
		return false

	case *ast.NewExpression:
		if js_new_callee_is_network(current.Callee) == true {
			return true
		}
		for _, argument := range current.ArgumentList {
			if js_expr_has_network(argument) == true {
				return true
			}
		}
		return false

	case *ast.DotExpression:
		return js_expr_has_network(current.Left)

	case *ast.BracketExpression:
		return js_expr_has_network(current.Left) || js_expr_has_network(current.Member)

	case *ast.AssignExpression:
		return js_expr_has_network(current.Left) || js_expr_has_network(current.Right)

	case *ast.BinaryExpression:
		return js_expr_has_network(current.Left) || js_expr_has_network(current.Right)

	case *ast.UnaryExpression:
		return js_expr_has_network(current.Operand)

	case *ast.AwaitExpression:
		return js_expr_has_network(current.Argument)

	case *ast.YieldExpression:
		return current.Argument != nil && js_expr_has_network(current.Argument)

	case *ast.ConditionalExpression:
		return js_expr_has_network(current.Test) ||
			js_expr_has_network(current.Consequent) ||
			js_expr_has_network(current.Alternate)

	case *ast.SequenceExpression:
		for _, child := range current.Sequence {
			if js_expr_has_network(child) == true {
				return true
			}
		}
		return false

	case *ast.ArrayLiteral:
		for _, child := range current.Value {
			if js_expr_has_network(child) == true {
				return true
			}
		}
		return false

	case *ast.ObjectLiteral:
		for _, property := range current.Value {
			if js_property_has_network(property) == true {
				return true
			}
		}
		return false

	case *ast.TemplateLiteral:
		for _, child := range current.Expressions {
			if js_expr_has_network(child) == true {
				return true
			}
		}
		return false

	case *ast.FunctionLiteral:
		return false

	case *ast.ArrowFunctionLiteral:
		return false

	case *ast.ClassLiteral:
		return false

	}

	return false

}

func js_binding_has_network(binding *ast.Binding) bool {

	if binding == nil {
		return false
	}

	return binding.Initializer != nil && js_expr_has_network(binding.Initializer)

}

func js_property_has_network(property ast.Property) bool {

	switch current := property.(type) {

	case *ast.PropertyShort:
		return current.Initializer != nil && js_expr_has_network(current.Initializer)

	case *ast.PropertyKeyed:
		return js_expr_has_network(current.Key) || js_expr_has_network(current.Value)

	case *ast.SpreadElement:
		return js_expr_has_network(current.Expression)

	}

	return false

}

func js_callee_is_network(callee ast.Expression) bool {

	switch current := callee.(type) {

	case *ast.Identifier:
		return js_network_callees[current.Name.String()]

	case *ast.DotExpression:
		return js_network_methods[current.Identifier.Name.String()]

	}

	return false

}

func js_new_callee_is_network(callee ast.Expression) bool {

	if identifier, ok := callee.(*ast.Identifier); ok == true {
		return js_network_constructors[identifier.Name.String()]
	}

	return false

}

func js_farble_return(block *ast.BlockStatement) string {

	if literal := js_first_return_literal(block); literal != "" {
		return "return " + literal + ";"
	}

	return "return true;"

}

func js_first_return_literal(block *ast.BlockStatement) string {

	for _, statement := range block.List {

		if current, ok := statement.(*ast.ReturnStatement); ok == true && current.Argument != nil {

			switch argument := current.Argument.(type) {

			case *ast.BooleanLiteral:
				return argument.Literal

			case *ast.NumberLiteral:
				return argument.Literal

			case *ast.StringLiteral:
				return argument.Literal

			case *ast.NullLiteral:
				return argument.Literal

			}

		}

	}

	return ""

}

func rewrite_js_imports(content []byte, resolve func(string) string) []byte {

	program, err := parser.ParseFile(nil, "", string(content), 0, parser.IsModule, parser.WithDisableSourceMaps)

	if err != nil {
		return content
	}

	rewrites := make([]js_spoof, 0)

	for _, statement := range program.Body {

		switch current := statement.(type) {

		case *ast.ImportDeclaration:

		case *ast.ExportDeclaration:
			if current.FromClause == nil {
				continue
			}

		default:
			continue

		}

		offset := int(statement.Idx0()) - 1
		open, close, raw_url := find_js_string_literal(content, offset)

		if raw_url == "" {
			continue
		}

		resolved := resolve(raw_url)

		if resolved == raw_url {
			continue
		}

		rewrites = append(rewrites, js_spoof{
			start: open + 1,
			end:   close - 1,
			body:  resolved,
		})

	}

	if len(rewrites) == 0 {
		return content
	}

	result := []byte(content)

	for index := len(rewrites) - 1; index >= 0; index-- {
		rewrite := rewrites[index]
		result = js_splice(result, rewrite.start, rewrite.end, rewrite.body)
	}

	return result

}

func find_js_string_literal(content []byte, from int) (int, int, string) {

	for index := from; index < len(content); index++ {

		if content[index] != '"' && content[index] != '\'' {
			continue
		}

		quote := content[index]
		open := index

		for index++; index < len(content); index++ {

			if content[index] == '\\' {
				index++
				continue
			}

			if content[index] == quote {
				return open, index + 1, string(content[open+1 : index])
			}

		}

	}

	return 0, 0, ""

}

