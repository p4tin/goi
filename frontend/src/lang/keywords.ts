// Goi's own short documentation for Go keywords, shown by Quick
// Documentation where gopls has nothing to say (R6.2).

export const KEYWORD_DOCS: Record<string, string> = {
  break: 'Ends the innermost `for`, `switch` or `select` statement. With a label (`break Outer`) it ends the labeled statement instead.',
  case: 'Introduces one clause of a `switch` or `select`. The clause runs when its expression matches (switch), or when its channel operation can proceed (select).',
  chan: 'Declares a channel type: `chan T` (send and receive), `chan<- T` (send only) or `<-chan T` (receive only). Create one with `make(chan T, size)`.',
  const: 'Declares named constants, evaluated at compile time. Inside a parenthesized group, `iota` counts from 0 for each line.\n\n```go\nconst (\n\tLow = iota\n\tHigh\n)\n```',
  continue: 'Skips the rest of the current iteration of the innermost `for` loop. With a label it continues the labeled loop.',
  default: 'The clause of a `switch` or `select` that runs when no other case matches. In a `select`, it makes the statement non-blocking.',
  defer: 'Schedules a function call to run when the surrounding function returns, including during a panic. Deferred calls run in last-in, first-out order; their arguments are evaluated immediately.\n\n```go\nf, err := os.Open(name)\nif err != nil {\n\treturn err\n}\ndefer f.Close()\n```',
  else: 'The alternative branch of an `if` statement; it may chain into another `if`.',
  fallthrough: 'As the last statement of a `switch` case, transfers control to the first statement of the next case without testing its expression.',
  for: 'The only loop statement. Forms: `for init; cond; post {}`, `for cond {}`, `for {}` (forever), `for i := range n {}` and `for k, v := range x {}` over slices, maps, strings, channels, integers and iterator functions.',
  func: 'Declares a function or method, or a function literal (closure). A receiver before the name makes it a method: `func (s *Server) Start() error`.',
  go: 'Starts a function call in a new goroutine, which runs concurrently with the caller. The statement does not wait for the call to finish.',
  goto: 'Transfers control to a label in the same function. It may not jump over variable declarations or into a block.',
  if: 'Runs a block when a boolean condition holds. A short statement may precede the condition: `if err := f(); err != nil { … }`.',
  import: 'Makes the exported names of other packages available in this file. Forms: `import "fmt"`, an alias `import str "strings"`, the blank import `import _ "embed"` and the dot import `import . "math"`.',
  interface: 'Declares an interface type: a set of methods (and, for constraints, type terms). A type implements an interface implicitly by having its methods.',
  map: 'Declares a hash map type `map[K]V`. Create one with `make(map[K]V)` or a literal. Reading a missing key yields the zero value; `v, ok := m[k]` reports presence.',
  package: 'Names the package a file belongs to. All files in a directory share one package name; `main` builds an executable.',
  range: 'Iterates in a `for` loop over a slice, array, string (by rune), map, channel, integer or iterator function, yielding index/key and value.',
  return: 'Ends the current function and optionally returns values. In functions with named results, a bare `return` returns their current values.',
  select: 'Waits on several channel operations and runs the first that can proceed; a `default` clause makes it non-blocking.\n\n```go\nselect {\ncase v := <-ch:\n\tuse(v)\ncase <-ctx.Done():\n\treturn ctx.Err()\n}\n```',
  struct: 'Declares a struct type: a sequence of named fields, optionally with tags. Embedded fields promote their fields and methods.',
  switch: 'Chooses one of several cases. Expression switches compare values; type switches (`switch v := x.(type)`) branch on dynamic type. Cases do not fall through unless `fallthrough` is used.',
  type: 'Declares a named type or a type alias (`type A = B`). With type parameters it declares a generic type: `type List[T any] struct { … }`.',
  var: 'Declares variables with an explicit type, an initializer, or both. Inside functions, `x := v` is the short form.',
};

export function keywordDoc(word: string): string | null {
  const doc = KEYWORD_DOCS[word];
  return doc ? `\`\`\`go\n${word}\n\`\`\`\n\n${doc}` : null;
}
