# Go Design Patterns Reference

> **Provenance**: Condensed and paraphrased (original wording; all code re-authored, none copied) from Refactoring.Guru — concepts: `https://refactoring.guru/design-patterns/<slug>`, Go examples: `https://refactoring.guru/design-patterns/<slug>/go/example`. Crawled 2026-07-29.
>
> **Scope**: The 22 GoF patterns restated for Go — interfaces and composition instead of inheritance. Standalone document.
>
> **How to use** — Planning agents: choose patterns via **Intent** and **Use when**. Refactoring agents: **Go sketch** is the target shape; read **Go notes** before introducing a pattern. Reviewer agents: **Go notes** flag un-idiomatic uses (e.g. Builder where functional options fit); cite anchors like `go-design-patterns.md#strategy`.

## Contents

- [Creational Patterns](#creational-patterns) (5)
- [Structural Patterns](#structural-patterns) (7)
- [Behavioral Patterns](#behavioral-patterns) (10)

## Creational Patterns

*How values come into existence — keeping call sites free of concrete types, assembly order, and lifetime decisions.*

### Abstract Factory

- **Intent**: Expose one interface that creates a whole family of related values, so a caller picks the family once and never names a concrete type.
- **Problem → Solution**: Code that constructs several related types inline can mix incompatible members of different families, and every new family edits every call site. Put the creation calls behind one factory interface with one implementation per family, so a single choice fixes all the products consistently.
- **Use when**: Products only make sense in matched sets (encoder + decoder, store + locker, driver + migrator); you want the variant chosen at exactly one wiring point.

**Go sketch**:

```go
type Encoder interface{ Encode(v any) ([]byte, error) }
type Decoder interface{ Decode(b []byte, v any) error }

type CodecFactory interface {
	NewEncoder() Encoder
	NewDecoder() Decoder
}

type jsonCodec struct{}

func (jsonCodec) NewEncoder() Encoder { return jsonEncoder{} }
func (jsonCodec) NewDecoder() Decoder { return jsonDecoder{} }

// protoCodec implements the same two methods for protobuf.

func CodecFor(name string) (CodecFactory, error) { /* "json" -> jsonCodec{} */ return nil, nil }
```

- **Go notes**: There are no abstract classes: the factory is a plain interface and each family is a small, usually empty struct. When a family has two or three members, a struct of function fields (`type Codec struct{ NewEncoder func() Encoder; NewDecoder func() Decoder }`) is lighter and just as type-safe. Any behavior shared by products comes from embedding a common struct, not from a base class.
- **Trade-offs**: + family members are guaranteed to match, and the variant decision lives in one place; − a lot of interfaces and types for what is often a single switch.
- **Related**: [Factory Method](#factory-method) — one product per call rather than a matched family; [Builder](#builder) — assembles one complex value step by step instead of handing back finished ones; [Prototype](#prototype) — copies a configured instance instead of constructing from scratch.

### Builder

- **Intent**: Construct a complex value through a sequence of named steps, so one procedure can produce several variants.
- **Problem → Solution**: A constructor taking every field, or a type per configuration, gets unreadable and resists extension. Move the steps onto a builder that accumulates state, validates it, and returns the finished value on a final call.
- **Use when**: Assembly has genuine ordering or validation rules, or several concrete products share one build sequence.

**Go sketch**:

```go
type Pipeline struct {
	source string
	stages []string
}

type PipelineBuilder interface {
	Source(string) PipelineBuilder
	Stage(string) PipelineBuilder
	Build() (Pipeline, error)
}

type batchBuilder struct{ p Pipeline }

func (b *batchBuilder) Source(s string) PipelineBuilder { b.p.source = s; return b }
func (b *batchBuilder) Stage(s string) PipelineBuilder  { b.p.stages = append(b.p.stages, s); return b }
func (b *batchBuilder) Build() (Pipeline, error)        { /* validate, then */ return b.p, nil }
```

- **Go notes**: For "many optional fields", functional options are the Go idiom — `New(src string, opts ...Option)` — and a Builder there is a code-review smell. Reserve Builder for staged assembly where steps must be ordered or validated together, or where several products share the sequence. A separate Director type rarely earns its keep; a plain function that drives the builder reads better.
- **Trade-offs**: + keeps ordering and validation out of the product, and one sequence yields variants; − an extra type plus a half-built state that can escape and be used by mistake.
- **Related**: [Abstract Factory](#abstract-factory) — returns finished family members rather than assembling one value; [Composite](#composite) — builders are the usual way to construct such trees; [Factory Method](#factory-method) — a single call with no intermediate state.

### Factory Method

- **Intent**: Put the choice of concrete type behind one call that returns an interface.
- **Problem → Solution**: Call sites that write `&Concrete{}` are pinned to that type and must all change when a variant appears. Route construction through one function that returns the interface, so adding a variant touches only that function.
- **Use when**: The concrete type depends on config, input, or environment; you want construction centralized so new variants land without editing callers.

**Go sketch**:

```go
type Notifier interface{ Notify(ctx context.Context, msg string) error }
type emailNotifier struct{ addr string }
type webhookNotifier struct{ url string }

func (e emailNotifier) Notify(ctx context.Context, msg string) error   { /* SMTP */ return nil }
func (w webhookNotifier) Notify(ctx context.Context, msg string) error { /* POST */ return nil }

func NewNotifier(kind, target string) (Notifier, error) { // the one place to extend
	switch kind {
	case "email":
		return emailNotifier{addr: target}, nil
	case "webhook":
		return webhookNotifier{url: target}, nil
	}
	return nil, fmt.Errorf("unknown notifier %q", kind)
}
```

- **Go notes**: Go has no virtual constructors, so the classic "creator subclass overrides the factory method" shape does not exist — embedding a struct does not override its methods. The everyday form is a package-level function returning an interface while the concrete types stay unexported. Introduce a creator *interface* only when the choice itself must be swappable at runtime, e.g. in tests.
- **Trade-offs**: + callers depend only on the interface, and variants are added in one place; − the concrete type is hidden from the compiler's help at call sites, and the switch becomes a hub everything depends on.
- **Related**: [Abstract Factory](#abstract-factory) — a matched family rather than one product; [Prototype](#prototype) — copies an existing instance instead of naming a kind; [Singleton](#singleton) — one instance rather than one construction path.

### Prototype

- **Intent**: Create new values by copying existing ones through a `Clone` method, without knowing their concrete type.
- **Problem → Solution**: Copying from outside means knowing every field, and unexported state is unreachable. Give each type a `Clone()` returning the shared interface so it copies itself, including any nested state it owns.
- **Use when**: Instances are expensive or fiddly to configure and you want variations of a prepared one; or you hold a tree of interface values that needs a deep copy.

**Go sketch**:

```go
type Widget interface{ Clone() Widget } // plus Render, Bounds, ... elided

type Label struct{ Text string }

func (l *Label) Clone() Widget { c := *l; return &c } // no reference fields: copy suffices

type Panel struct{ Children []Widget }

func (p *Panel) Clone() Widget {
	c := &Panel{Children: make([]Widget, len(p.Children))}
	for i, ch := range p.Children {
		c.Children[i] = ch.Clone() // deep: never share the backing array
	}
	return c
}
```

- **Go notes**: There is no copy constructor and no deep-copy builtin. `*p` copies the struct but shares every slice, map, pointer, and channel inside it, so `Clone` must rebuild those by hand — this is where the bugs live. For plain value types with no reference fields, assignment already is the clone and the pattern adds nothing.
- **Trade-offs**: + duplicates unexported state and whole trees without naming concrete types; − every type maintains its own `Clone`, shallow/deep mistakes fail silently, and cycles need extra bookkeeping.
- **Related**: [Factory Method](#factory-method) — builds from a kind rather than from an instance; [Memento](#memento) — a snapshot kept for restore rather than a live duplicate; [Composite](#composite) — the tree shape that makes deep cloning necessary.

### Singleton

- **Intent**: Guarantee a type has exactly one instance and give the program one way to reach it.
- **Problem → Solution**: Some resources must not be duplicated — a connection pool, a metrics registry — yet are needed in many places. Hide construction behind an accessor that builds the value once and returns the same pointer forever after.
- **Use when**: The resource is genuinely process-wide and safe to share, and its construction must happen at most once.

**Go sketch**:

```go
type Registry struct {
	mu     sync.Mutex
	counts map[string]int
}

var (
	registryOnce sync.Once
	registry     *Registry
)

func Shared() *Registry {
	registryOnce.Do(func() {
		registry = &Registry{counts: make(map[string]int)}
	})
	return registry
}
```

- **Go notes**: `sync.Once`, or a package-level `var x = newX()` run at program init, replaces the private-constructor trick; hand-written double-checked locking is unnecessary and easy to get wrong. The instance must itself be safe for concurrent use. A shared global is a hidden dependency tests cannot replace — prefer constructing once in `main` and injecting it, keeping an accessor only where injection is impractical.
- **Trade-offs**: + one initialization, one instance, reachable from anywhere; − global mutable state, initialization-order coupling, and tests that cannot isolate or substitute it.
- **Related**: [Facade](#facade) — often exposed as one shared entry point; [Flyweight](#flyweight) — many shared instances keyed by state, not a single one; [Abstract Factory](#abstract-factory) — the factory itself is a common thing to make process-wide.

## Structural Patterns

*How types are composed into larger structures — wrapping, layering, and sharing without inheritance.*

### Adapter

- **Intent**: Let a type you cannot change satisfy the interface your code already expects, by translating between the two.
- **Problem → Solution**: A vendor client, legacy package, or generated stub exposes the wrong method shapes, and rewriting callers around it spreads the mismatch. Write a thin type that implements your interface and forwards each call to the foreign API, converting arguments and errors at that boundary.
- **Use when**: Integrating third-party or legacy code; unifying two implementations behind one interface; keeping foreign types out of your core packages.

**Go sketch**:

```go
// MetricSink is what our code depends on.
type MetricSink interface{ Record(name string, v float64) }

// vendorClient is the third-party API we cannot change.
type vendorClient struct{ /* ... */ }

func (c *vendorClient) Push(series string, points []float64) error { /* ... */ return nil }

type vendorSink struct{ c *vendorClient } // the adapter

func (a vendorSink) Record(name string, v float64) {
	_ = a.c.Push(name, []float64{v}) // translate one call shape into the other
}

// var sink MetricSink = vendorSink{c: newVendorClient()}
```

- **Go notes**: Because interfaces are satisfied implicitly, the target interface should be declared by the consumer and kept small — one or two methods make adapters nearly free. When the interface has a single method, the `http.HandlerFunc` trick (a named func type with a method on it) adapts a plain function without any struct. Errors dropped or invented at the boundary are the usual review finding.
- **Trade-offs**: + isolates a foreign API behind one file that is easy to fake in tests; − one more indirection, and impedance mismatches (errors, contexts, batching) get papered over silently.
- **Related**: [Decorator](#decorator) — same interface in and out, adding behavior rather than translating; [Proxy](#proxy) — same interface, controlling access; [Facade](#facade) — simplifies a whole subsystem rather than converting one type.

### Bridge

- **Intent**: Split a design along two independent axes so each can grow without multiplying types.
- **Problem → Solution**: Covering two dimensions with one type hierarchy (chart kind × output format) needs a type per combination, and every new value on either axis multiplies the rest. Make one axis an interface, hold it as a field on the other, and combine them at construction.
- **Use when**: Two orthogonal dimensions of variation exist; you want to pick the implementation at runtime, or swap it per environment.

**Go sketch**:

```go
// Canvas is the implementation side.
type Canvas interface {
	Line(x1, y1, x2, y2 int)
	Text(x, y int, s string)
}

type Chart struct{ c Canvas } // abstraction side: holds an implementation

type BarChart struct{ Chart }
func (b BarChart) Draw(vals []int) { /* b.c.Line(...) per bar */ }

type LineChart struct{ Chart }
func (l LineChart) Draw(vals []int) { /* l.c.Line(...) between points */ }

// var ch = BarChart{Chart{c: svgCanvas{}}} // any chart x any canvas
```

- **Go notes**: In Go this is usually just "hold an interface field", which is the default way to build anything — so the pattern is rarely named aloud. Embedding a shared struct (`Chart`) gives the refined types the field and helpers without inheritance, but embedding never overrides: each concrete chart defines its own `Draw`. If one axis has a single implementation, drop the interface until a second appears.
- **Trade-offs**: + the two axes grow additively instead of multiplicatively, and implementations swap at runtime; − an interface introduced before a second implementation exists is pure ceremony.
- **Related**: [Strategy](#strategy) — same structure, but the field is an algorithm chosen per call rather than a permanent implementation half; [Abstract Factory](#abstract-factory) — supplies matched implementation objects; [Adapter](#adapter) — retrofits compatibility rather than planning for it.

### Composite

- **Intent**: Give leaves and containers the same interface so a tree can be handled like a single value.
- **Problem → Solution**: Code that walks a nested structure must keep asking "is this a group or an item?", and that test spreads into every operation. Have both kinds satisfy one interface, and let containers implement each operation by delegating to their children.
- **Use when**: The domain is genuinely tree-shaped (checks, filesystem entries, menus, expressions) and operations apply uniformly at every level.

**Go sketch**:

```go
type Check interface{ Run(ctx context.Context) error }

type Step func(context.Context) error // leaf
func (s Step) Run(ctx context.Context) error { return s(ctx) }

type Suite struct{ children []Check } // composite: holds the same interface

func (s *Suite) Add(c Check) { s.children = append(s.children, c) }
func (s *Suite) Run(ctx context.Context) error {
	for _, c := range s.children {
		if err := c.Run(ctx); err != nil {
			return err
		}
	}
	return nil
}
```

- **Go notes**: One interface plus a `[]Iface` field on the container is the whole pattern; a leaf can be a named func type rather than a struct. Watch receivers: if the composite mutates (`Add`), its methods need pointer receivers and callers must store `*Suite` in the interface, or the additions vanish. Deep trees plus recursion mean unbounded stack — take a `context.Context` and check for cancellation on long walks.
- **Trade-offs**: + one code path for one item and for a whole tree; recursion replaces type tests; − the shared interface drifts toward the union of leaf and container needs, and leaf-only methods start returning errors.
- **Related**: [Decorator](#decorator) — also wraps the same interface, but adds behavior to one child instead of aggregating many; [Visitor](#visitor) — adds new operations over an existing tree; [Iterator](#iterator) — walks the tree without exposing its shape.

### Decorator

- **Intent**: Add behavior to a value by wrapping it in another value with the same interface.
- **Problem → Solution**: Optional behaviors — logging, retries, metrics, caching — multiply into flags and conditionals inside the core type, or into a type per combination. Wrap the original in a type satisfying the same interface that does its extra work before or after delegating, and stack the wrappers.
- **Use when**: Cross-cutting behavior should compose per call site; you want each concern testable alone; the wrapped type is closed to modification.

**Go sketch**:

```go
type Fetcher interface{ Fetch(ctx context.Context, key string) ([]byte, error) }

type FetcherFunc func(ctx context.Context, key string) ([]byte, error)

func (f FetcherFunc) Fetch(ctx context.Context, key string) ([]byte, error) { return f(ctx, key) }

func WithRetry(next Fetcher, n int) Fetcher { // same interface in, same interface out
	return FetcherFunc(func(ctx context.Context, key string) ([]byte, error) {
		b, err := next.Fetch(ctx, key)
		for i := 1; err != nil && i < n; i++ {
			b, err = next.Fetch(ctx, key)
		}
		return b, err
	})
}
// wiring: var f Fetcher = WithRetry(WithLogging(diskFetcher{dir: "/cache"}), 3)
```

- **Go notes**: `func(http.Handler) http.Handler` middleware is the canonical Go form, and the same `With…` convention reads well for any single-method interface. Wrapping is transparent only for the declared methods: a wrapper hides any other method or concrete-type assertion the inner value supported, which breaks things like `http.Flusher` unless explicitly forwarded. Order matters — the outermost wrapper runs first.
- **Trade-offs**: + concerns stay separate and compose per call site without touching the core; − stack traces and debugging gain layers, order bugs are quiet, and unwrapped capabilities disappear.
- **Related**: [Proxy](#proxy) — same shape, but the point is controlling access, not enriching behavior; [Adapter](#adapter) — changes the interface instead of preserving it; [Chain of Responsibility](#chain-of-responsibility) — a chain where a link may stop the request instead of always delegating.

### Facade

- **Intent**: Put one small interface in front of a large subsystem so callers use it without learning its parts.
- **Problem → Solution**: Callers otherwise construct several collaborating types in the right order and repeat that dance everywhere. Expose one type (or one function) that owns the parts and offers the few operations callers actually need.
- **Use when**: A subsystem has many moving pieces but few real use cases; you are layering a system and want a controlled seam between layers.

**Go sketch**:

```go
type Publisher struct { // the package's entire exported surface
	store  *blobStore
	index  *searchIndex
	notify *mailer
}

func New(cfg Config) *Publisher { /* wire the three subsystems in order */ return nil }

func (p *Publisher) Publish(ctx context.Context, d Doc) error {
	id, err := p.store.Put(ctx, d.Body)
	if err != nil {
		return err
	}
	p.index.Add(id, d.Title) // in-memory, cannot fail
	return p.notify.Announce(ctx, d.Owner, id)
}
```

- **Go notes**: In Go this is mostly a packaging decision — the package boundary is the facade: a small exported API over unexported collaborators, with no interface unless callers must substitute the whole subsystem in tests. Because the facade owns construction order, it is the natural home for `Close` and other lifecycle handling. A facade that forwards calls one-for-one is noise, and reviewers should say so.
- **Trade-offs**: + callers learn one small API and stay decoupled from internal wiring; − it accretes methods until it is a god object, and it hides options users eventually need.
- **Related**: [Adapter](#adapter) — converts one type's interface rather than simplifying many; [Singleton](#singleton) — facades are often the thing people make process-wide; [Mediator](#mediator) — coordinates peers that talk back, where a facade only forwards.

### Flyweight

- **Intent**: Share one immutable copy of repeated state across many objects instead of duplicating it.
- **Problem → Solution**: Huge numbers of similar values each carry identical data, and the duplication dominates memory. Split the value into the shared immutable part and the per-instance part, and hand out one shared pointer per distinct shared value from a cache.
- **Use when**: Object counts are large enough that memory is a measured problem, and a real subset of the state is identical and immutable.

**Go sketch**:

```go
type Style struct { // intrinsic state: immutable, shared by thousands of cells
	FG, BG uint8
	Bold   bool
}

type Cell struct { // extrinsic state per cell + a pointer to one shared Style
	R     rune
	Style *Style
}

var styles sync.Map // map[Style]*Style; safe for concurrent interning

func Intern(s Style) *Style {
	p, _ := styles.LoadOrStore(s, &s)
	return p.(*Style)
}
```

- **Go notes**: The shared value must be treated as immutable — hand out a pointer only if nothing can write through it, or share by value and skip the pattern. The cache needs a mutex or `sync.Map`, and it never shrinks unless entries are evicted, so an unbounded intern table is a memory leak wearing an optimization's clothes. Confirm with a profile first; Go's allocator makes many small structs cheaper than expected.
- **Trade-offs**: + large, measurable memory savings when duplication is real; − an extra lookup on every construction, plus lifetime and mutability hazards around shared values.
- **Related**: [Singleton](#singleton) — one instance overall rather than one per distinct value; [Composite](#composite) — flyweights commonly serve as its leaves; [Prototype](#prototype) — copies state where flyweight refuses to copy it.

### Proxy

- **Intent**: Stand in for another value behind the same interface, controlling when and whether calls reach it.
- **Problem → Solution**: A real backend is expensive to create, remote, or must be restricted, and putting that logic in callers or in the backend itself spreads it. Insert a type satisfying the same interface that decides — lazily construct, cache, authorize, rate-limit — before delegating.
- **Use when**: Construction should be deferred until first use; access needs checks, quotas, or caching; a remote service needs a local stand-in.

**Go sketch**:

```go
type Index interface{ Lookup(ctx context.Context, q string) ([]string, error) }

type lazyIndex struct { // stand-in for a remote index
	addr string
	once sync.Once
	real Index
}

func (l *lazyIndex) Lookup(ctx context.Context, q string) ([]string, error) {
	l.once.Do(func() { l.real = dialIndex(l.addr) }) // expensive; deferred to first call
	return l.real.Lookup(ctx, q)
}

func NewIndex(addr string) Index { return &lazyIndex{addr: addr} } // callers see only Index
```

- **Go notes**: Structurally identical to [Decorator](#decorator); the difference is intent, so name the type for what it controls (`lazyIndex`, `cachedRepo`, `throttledClient`). State inside a proxy is shared by every caller, so guard it — `sync.Once` for lazy init, a mutex or `singleflight` for caches. A proxy that swallows errors from the deferred construction turns a startup failure into a mysterious runtime one.
- **Trade-offs**: + adds laziness, caching, or access control without touching either side; − hides real cost and failure behind an innocuous call, and adds a concurrency-sensitive place to get wrong.
- **Related**: [Decorator](#decorator) — same wrapping, aimed at enriching behavior; [Adapter](#adapter) — deliberately presents a different interface; [Facade](#facade) — one simpler entry point over many types rather than a stand-in for one.

## Behavioral Patterns

*How responsibility and control flow are distributed — who decides, who is notified, and who does the work.*

### Chain of Responsibility

- **Intent**: Pass a request along a series of handlers, each free to deal with it, transform it, or hand it on.
- **Problem → Solution**: One function accumulates guard clauses — auth, quota, validation, dedup — until nothing can be reordered, reused, or tested alone. Give each step its own type behind a common interface holding the next step, so composition sets the order and any step may stop the request.
- **Use when**: Pre-processing steps vary per route, tenant, or message kind; the order must be configurable; steps deserve individual tests.

**Go sketch**:

```go
type Stage interface{ Handle(ctx context.Context, r *Req) error }

type auth struct{ next Stage } // each link decides whether to continue

func (a auth) Handle(ctx context.Context, r *Req) error {
	if r.Token == "" {
		return ErrUnauthorized // stop: the rest of the chain never runs
	}
	return a.next.Handle(ctx, r)
}

type quota struct{ next Stage }

func (q quota) Handle(ctx context.Context, r *Req) error { /* count */ return q.next.Handle(ctx, r) }

// wiring: var root Stage = auth{next: quota{next: finalStage{}}}
```

- **Go notes**: The everyday Go form is middleware — `func(Handler) Handler` closures composed once at startup, as in `net/http`. An explicit `next` field is worth it only when links carry state. Two recurring review findings: a link that forgets to call `next` (requests vanish silently) and a nil `next` at the tail — end the chain with a real terminal handler rather than a nil check in every link.
- **Trade-offs**: + steps stay independent, reorderable, and unit-testable; − control flow lives in the wiring rather than the code, and a request can fall off the end unhandled.
- **Related**: [Decorator](#decorator) — always delegates, where a chain link may stop the request; [Command](#command) — makes the request an object instead of the handling a chain; [Mediator](#mediator) — a hub decides routing instead of each link.

### Command

- **Intent**: Turn an operation plus its arguments into a value that can be stored, passed, queued, retried, or reversed.
- **Problem → Solution**: Code that calls operations directly cannot defer, record, or undo them, and every new trigger duplicates the call. Wrap each operation in a type with a fixed method set, so an invoker can hold it, run it later, log it, and ask it to undo.
- **Use when**: You need undo/redo, a work queue, retries, or an audit trail of what was requested; several triggers issue the same operation.

**Go sketch**:

```go
type Command interface {
	Do(ctx context.Context) error
	Undo(ctx context.Context) error
}

type Rename struct{ from, to string } // one concrete command

func (r Rename) Do(ctx context.Context) error   { return os.Rename(r.from, r.to) }
func (r Rename) Undo(ctx context.Context) error { return os.Rename(r.to, r.from) }

type History struct{ done []Command } // invoker: queue, log, and undo stack

func (h *History) Run(ctx context.Context, c Command) error { /* Do, push on success */ return nil }

func (h *History) UndoLast(ctx context.Context) error { /* pop, then Undo */ return nil }
```

- **Go notes**: A closure or a `func(context.Context) error` usually suffices. Struct commands earn their keep when you need undo, serialization onto a durable queue, or a readable audit log — a closure's captured state cannot be inspected, logged, or persisted. If commands outlive the process, give them exported, serializable fields and a kind tag.
- **Trade-offs**: + operations become first-class values: queueable, retryable, undoable, loggable; − a type per operation, and `Undo` that silently rots out of sync with `Do`.
- **Related**: [Memento](#memento) — snapshots the state an undo restores, instead of inverting the operation; [Strategy](#strategy) — varies how work is done, not what was requested; [Chain of Responsibility](#chain-of-responsibility) — routes a request through handlers rather than reifying it.

### Iterator

- **Intent**: Expose a collection's elements one at a time without revealing how it stores them.
- **Problem → Solution**: Traversal code that knows the internal layout must change whenever the layout does, and each structure grows its own bespoke loop. Hand out a traversal function or cursor; callers consume elements and never see the tree, the page cursor, or the file handle behind them.
- **Use when**: The structure is not a plain slice (tree, paged API, streamed file); several traversal orders exist; the sequence is large or unbounded.

**Go sketch**:

```go
type Tree struct {
	Val         int
	Left, Right *Tree
}

// All yields values in order; callers write: for v := range t.All() { ... }
func (t *Tree) All() iter.Seq[int] {
	return func(yield func(int) bool) { t.walk(yield) }
}

func (t *Tree) walk(yield func(int) bool) bool {
	if t == nil {
		return true
	}
	return t.Left.walk(yield) && yield(t.Val) && t.Right.walk(yield) // false = consumer stopped
}
```

- **Go notes**: Since Go 1.23 the idiomatic form is range-over-func: return `iter.Seq[T]` or `iter.Seq2[K, V]` and let callers use `for x := range s`. `yield` returning false means the consumer broke out — propagate it or the walk runs on. Channel-based iterators leak a goroutine whenever the consumer stops early; avoid them unless the producer is genuinely concurrent and takes a context. If the data is already a slice, return the slice.
- **Trade-offs**: + hides the structure, supports multiple orders and early exit, and streams without materializing; − yield plumbing obscures simple cases, and mutation during iteration is undefined unless you document it.
- **Related**: [Composite](#composite) — the tree an iterator typically walks; [Visitor](#visitor) — takes the operation to the elements rather than handing elements out; [Factory Method](#factory-method) — how a collection hands back its iterator.

### Mediator

- **Intent**: Route interaction between components through one coordinator so they never reference each other.
- **Problem → Solution**: When every component holds pointers to its peers, the group becomes one tangled unit that cannot be tested, reused, or changed in isolation. Give each component a reference to a single coordinator and let it decide who is told what.
- **Use when**: Several peers must react to each other's state; the interaction rules change more often than the components; you want to reuse a component elsewhere without its peers.

**Go sketch**:

```go
type Worker interface {
	Name() string
	Start(job string)
}

type Dispatcher struct { // the mediator: workers talk to it, never to each other
	workers []Worker
	busy    map[string]bool
}

func (d *Dispatcher) Register(w Worker) { d.workers = append(d.workers, w) }

func (d *Dispatcher) Done(w Worker) {
	d.busy[w.Name()] = false
	/* pick an idle worker and Start the next queued job */
}
```

- **Go notes**: A plain coordinator struct with methods is enough; no `Mediator` interface unless the coordination itself gets swapped. For concurrent components, the natural Go mediator is a goroutine that owns the state and receives requests over a channel, which removes shared locking altogether. Watch for the coordinator needing each component's internals — that means the split is in the wrong place.
- **Trade-offs**: + components stay independent and the interaction rules live in one readable place; − the coordinator drifts into a god object and becomes a serialization bottleneck.
- **Related**: [Observer](#observer) — components broadcast to subscribers instead of a hub deciding; [Facade](#facade) — one-way simplification with no coordination or callbacks; [Command](#command) — what a mediator often queues and dispatches.

### Memento

- **Intent**: Capture a value's internal state so it can be restored later, without exposing that state.
- **Problem → Solution**: Undo, rollback, and checkpointing all need a copy of internals, and exporting those fields so a caretaker can save them destroys the type's invariants. Have the type produce an opaque snapshot only it can interpret, and let a caretaker store it and hand it back.
- **Use when**: You need undo/redo, transactional rollback, or "restore last known good"; the state is unexported and must stay that way.

**Go sketch**:

```go
type Editor struct {
	text   string
	cursor int
}

// Snapshot is opaque: unexported fields, no methods, only Editor can read it.
type Snapshot struct {
	text   string
	cursor int
}

func (e *Editor) Save() Snapshot     { return Snapshot{text: e.text, cursor: e.cursor} }
func (e *Editor) Restore(s Snapshot) { e.text, e.cursor = s.text, s.cursor }

type History struct{ stack []Snapshot } // caretaker: stores, never inspects
```

- **Go notes**: Value structs snapshot for free — assignment copies them — so the pattern reduces to "return a struct with unexported fields from `Save`". As with Prototype, any slice, map, or pointer inside must be copied explicitly, or the snapshot mutates along with the original. Keeping the snapshot type free of exported fields and setters is what makes it a memento rather than a leaked struct.
- **Trade-offs**: + restores state without leaking internals, and history logic stays outside the type; − memory grows with snapshot count, and reference fields make "copies" deceptive.
- **Related**: [Command](#command) — the undoable operation that holds the snapshots; [Prototype](#prototype) — a live duplicate meant for use, not a stored state record; [Iterator](#iterator) — traversal position is a common thing to snapshot.

### Observer

- **Intent**: Let a producer notify a changing set of interested parties without knowing who they are.
- **Problem → Solution**: Wiring a producer directly to each consumer means editing it for every new listener, while polling burns work to discover nothing changed. Let listeners register a callback or channel, and have the producer fan an event out to whoever is registered at that moment.
- **Use when**: Several unrelated parts must react to a state change; the listener set varies at runtime; producers should stay unaware of consumers.

**Go sketch**:

```go
type Event struct{ Kind, ID string }

type Bus struct {
	mu   sync.Mutex
	subs map[*func(Event)]struct{} // observers are plain callbacks
}

func (b *Bus) Subscribe(fn func(Event)) (cancel func()) { // caller owns the lifetime
	h := &fn
	b.mu.Lock()
	b.subs[h] = struct{}{}
	b.mu.Unlock()
	return func() { b.mu.Lock(); delete(b.subs, h); b.mu.Unlock() }
}

func (b *Bus) Publish(e Event) { /* snapshot subs under lock, then call each */ }
```

- **Go notes**: Callback registration returning an unsubscribe func is the plain form; channels suit subscribers that are goroutines, but then buffering and slow-consumer policy become your problem — blocking, dropping, and unbounded buffering are three different bugs. Never invoke subscribers while holding the lock: snapshot the set, unlock, then call. Delivery order is unspecified, so never encode dependencies in it.
- **Trade-offs**: + producers and consumers evolve independently and subscriptions are dynamic; − control flow gets hard to follow, and forgotten unsubscribes leak both memory and goroutines.
- **Related**: [Mediator](#mediator) — a hub that decides who acts, rather than a broadcast to whoever listens; [Chain of Responsibility](#chain-of-responsibility) — one ordered path where a link can stop, not a fan-out; [Command](#command) — what handlers often enqueue in response.

### State

- **Intent**: Move state-dependent behavior into one type per state so the context delegates instead of branching.
- **Problem → Solution**: A type with a `status` field grows a switch in every method, and transition rules get duplicated until they disagree. Give each state its own type behind a shared interface, hold the current one in the context, and let states name their successors.
- **Use when**: Behavior differs substantially per state; transitions have rules worth naming and testing; the same switch keeps reappearing.

**Go sketch**:

```go
type State interface{ Next(ev Event) State } // a state picks its own successor

type Idle struct{}

func (Idle) Next(ev Event) State {
	if ev == Start {
		return Running{}
	}
	return Idle{}
}

type Running struct{}

func (Running) Next(ev Event) State { /* Done -> Finished{}; Fail -> Retrying{} */ return Running{} }

// context: type Job struct{ st State }; on an event: j.st = j.st.Next(ev)
```

- **Go notes**: The context holds a `State` interface field and reassigns it; with no inheritance, shared behavior comes from embedding a common struct or calling a helper. Func-typed states — `type stateFn func(Event) stateFn`, the shape used by the standard library's lexers — are idiomatic when a state's whole job is picking the next one. For three states and two transitions, an enum plus a switch is genuinely better; say so in review.
- **Trade-offs**: + transitions become explicit and each state is testable in isolation; − the machine's overall shape is scattered across types, and small machines get harder to read, not easier.
- **Related**: [Strategy](#strategy) — chosen from outside and unaware of its siblings, where states switch themselves; [Command](#command) — the events a machine consumes; [Bridge](#bridge) — same "hold an interface" shape, permanent rather than shifting.

### Strategy

- **Intent**: Put interchangeable algorithm variants behind one interface so callers can swap them at runtime.
- **Problem → Solution**: A type grows conditional branches selecting between algorithm variants, and every new variant touches it. Extract each variant into its own type satisfying a small interface; the context holds and calls the interface.
- **Use when**: Several variants of one behavior exist; "mode" conditionals keep spreading; you want variants testable in isolation.

**Go sketch**:

```go
type Router interface {
	Route(from, to Point) []Point
}

type Fastest struct{}
func (Fastest) Route(from, to Point) []Point { /* shortest time */ return nil }

type Scenic struct{}
func (Scenic) Route(from, to Point) []Point { /* nicest views */ return nil }

type Navigator struct{ r Router }

func NewNavigator(r Router) *Navigator       { return &Navigator{r: r} }
func (n *Navigator) Plan(a, b Point) []Point { return n.r.Route(a, b) }
```

- **Go notes**: For single-method strategies a func type (`type RouteFunc func(a, b Point) []Point`) is more idiomatic than an interface; use the interface only when strategies carry state or multiple methods. Inject via constructor; add a setter only if the strategy changes mid-flight.
- **Trade-offs**: + swaps behavior without touching the context; variants isolated for testing; − needless indirection when variants are few and stable.
- **Related**: [State](#state) — states switch themselves, strategies are chosen from outside; [Command](#command); [Template Method](#template-method) — fixed skeleton with varying steps instead of whole-algorithm swap.

### Template Method

- **Intent**: Fix the skeleton of an algorithm in one place and let callers supply only the steps that vary.
- **Problem → Solution**: Several routines repeat the same order of operations while differing at two or three points, and the shared parts drift apart as each is edited. Write the sequence once and have it call out to steps the caller provides.
- **Use when**: Variants share an invariant order (fetch → transform → store); the order carries correctness weight and should not be re-implemented per variant.

**Go sketch**:

```go
type Steps interface { // the varying parts; the skeleton never changes
	Fetch(ctx context.Context) ([]byte, error)
	Transform(b []byte) []byte
	Store(ctx context.Context, b []byte) error
}

// Run is the template method: fixed order, delegated steps.
func Run(ctx context.Context, s Steps) error {
	raw, err := s.Fetch(ctx)
	if err != nil {
		return err
	}
	return s.Store(ctx, s.Transform(raw))
}

// A CSV importer supplies its own Fetch/Transform/Store; Run never changes.
```

- **Go notes**: Go has no method overriding, so the classic shape does not translate: embedding a struct does not let the outer type replace a method the embedded one calls internally — the inner method always calls its own. Take a small steps interface, or hook funcs on an options struct (`Transform func([]byte) []byte`) with sane defaults. A plain function taking one or two callbacks is usually the clearest version.
- **Trade-offs**: + the shared order lives in one place and each variant stays small; − a rigid skeleton, and hook signatures that must anticipate every variant's needs.
- **Related**: [Strategy](#strategy) — swaps the whole algorithm instead of steps inside a fixed one; [Factory Method](#factory-method) — a template whose varying step is construction; [Chain of Responsibility](#chain-of-responsibility) — steps chosen by composition, and any may stop the run.

### Visitor

- **Intent**: Add an operation across a set of types without editing them, by letting each type dispatch to the operation.
- **Problem → Solution**: Each new analysis over a node hierarchy otherwise adds a method to every node type, piling unrelated concerns onto the data. Define an operation interface with one method per node type and give nodes an `Accept` that calls the matching one, so new operations arrive as new types.
- **Use when**: Node types are stable but operations keep multiplying (evaluate, print, typecheck, price); an operation needs its own state across the whole traversal.

**Go sketch**:

```go
type Node interface{ Accept(v Visitor) }

type Visitor interface {
	VisitFile(*File)
	VisitDir(*Dir)
}

type File struct{ Size int64 }
func (f *File) Accept(v Visitor) { v.VisitFile(f) } // dispatch on the concrete type

type Dir struct{ Kids []Node }
func (d *Dir) Accept(v Visitor) { v.VisitDir(d) }

type sizer struct{ total int64 } // one operation over the whole tree
func (s *sizer) VisitFile(f *File) { s.total += f.Size }
func (s *sizer) VisitDir(d *Dir)   { for _, k := range d.Kids { k.Accept(s) } }
```

- **Go notes**: There is no method overloading and no double dispatch, so each node type needs its own visitor method name plus an `Accept`. Most Go code skips the ceremony and writes `switch n := n.(type)` inside the operation — shorter and honest when the node types live in one package. Neither form gets a compiler warning for a missed type, so give the type switch a `default` that fails loudly.
- **Trade-offs**: + new operations without touching node types, with a natural home for traversal state; − adding a node type breaks every visitor, and the `Accept` boilerplate buys little over a type switch.
- **Related**: [Composite](#composite) — the tree a visitor traverses; [Iterator](#iterator) — hands elements out instead of bringing the operation in; [Strategy](#strategy) — one swappable algorithm rather than one per element type.
