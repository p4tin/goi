// @vitest-environment happy-dom
import { describe, expect, it } from 'vitest';
import { counterpartFunc, counterpartPath, enclosingFunc, findFuncLine, NavHistory } from '../src/lang/navigation';

const e = (path: string, line: number) => ({ path, line, col: 1 });

describe('NavHistory', () => {
  it('goes back and forward across jumps', () => {
    const h = new NavHistory();
    h.push(e('a', 1)); // jump a:1 -> b:5
    h.push(e('b', 5)); // jump b:5 -> c:9
    expect(h.back(e('c', 9))).toEqual(e('b', 5));
    expect(h.back(e('b', 5))).toEqual(e('a', 1));
    expect(h.back(e('a', 1))).toBeNull();
    expect(h.forward()).toEqual(e('b', 5));
    expect(h.forward()).toEqual(e('c', 9));
    expect(h.forward()).toBeNull();
  });
  it('drops forward history on a new jump', () => {
    const h = new NavHistory();
    h.push(e('a', 1));
    h.push(e('b', 2));
    h.back(e('c', 3)); // at b:2
    h.push(e('b', 2)); // jump b:2 -> d:4
    expect(h.forward()).toBeNull();
    expect(h.back(e('d', 4))).toEqual(e('b', 2));
    expect(h.back(e('b', 2))).toEqual(e('a', 1));
  });
  it('dedupes consecutive entries on the same line and caps size', () => {
    const h = new NavHistory(3);
    h.push(e('a', 1));
    h.push(e('a', 1));
    expect(h.size).toBe(1);
    for (let i = 2; i < 10; i++) h.push(e('x', i));
    expect(h.size).toBe(3);
  });
});

describe('go to test', () => {
  it('maps file names', () => {
    expect(counterpartPath('/p/server.go')).toBe('/p/server_test.go');
    expect(counterpartPath('/p/server_test.go')).toBe('/p/server.go');
    expect(counterpartPath('/p/readme.md')).toBeNull();
  });
  it('finds the enclosing function or method', () => {
    const src = 'package p\n\nfunc Foo() {\n\tx()\n}\n\nfunc (s *Server[T]) Start(ctx context.Context) error {\n\treturn nil\n}\n';
    expect(enclosingFunc(src, src.indexOf('x()'))).toEqual({ name: 'Foo', recv: undefined });
    expect(enclosingFunc(src, src.indexOf('return'))).toEqual({ name: 'Start', recv: 'Server' });
    expect(enclosingFunc(src, 3)).toBeNull();
  });
  it('maps function names both ways', () => {
    expect(counterpartFunc({ name: 'Foo' }, false)).toEqual({ name: 'TestFoo' });
    expect(counterpartFunc({ name: 'Start', recv: 'Server' }, false)).toEqual({ name: 'TestServer_Start' });
    expect(counterpartFunc({ name: 'TestFoo' }, true)).toEqual({ name: 'Foo' });
    expect(counterpartFunc({ name: 'TestServer_Start' }, true)).toEqual({ name: 'Start', recv: 'Server' });
    expect(counterpartFunc({ name: 'TestFoo_emptyInput' }, true)).toEqual({ name: 'Foo' });
    expect(counterpartFunc({ name: 'BenchmarkParse' }, true)).toEqual({ name: 'Parse' });
  });
  it('locates functions and methods', () => {
    const src = 'package p\n\nfunc Start() {}\n\nfunc (s *Server) Start() {}\n\nfunc TestFoo(t *testing.T) {}\n';
    expect(findFuncLine(src, { name: 'Start', recv: 'Server' })).toBe(5);
    expect(findFuncLine(src, { name: 'Start' })).toBe(3);
    expect(findFuncLine(src, { name: 'TestFoo' })).toBe(7);
    expect(findFuncLine(src, { name: 'Missing' })).toBe(0);
  });
});
