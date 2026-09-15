# Curlew local examples cookbook

The cookbook contains fourteen complete examples for the current local CLI.
Every scenario uses the repository's Mudflat fixture API, public fixture
credentials where needed, and checked-in data/query files. No cloud account is
required. The site is separate from the retained platform dashboard in `web/`.

## Run an example

Use an authenticated checkout. Build the complete app with Go 1.24+ and Node.js 22+:

```bash
./scripts/build-ui.sh
go build -o curlew ./cmd/curlew
go build -o mudflat ./testapi/cmd/mudflat
export PATH="$PWD:$PATH"
./mudflat serve --port 18080
```

Keep that terminal running. In another terminal at the repository root, add the
checkout to PATH again and choose a scenario's **Prerequisites** and **Run it**
blocks. The defaults use `http://127.0.0.1:18080` and `ws://127.0.0.1:18080`.
A fresh RUN_ID isolates state between runs. Stop Mudflat with Ctrl+C when done.

For example:

```bash
export PATH="$PWD:$PATH"
export MUDFLAT_URL=http://127.0.0.1:18080
export RUN_ID="cookbook-$(date +%s)-$$"
curlew run testapi/collections/20-extraction.yaml \
  --var mud="$MUDFLAT_URL" --var run="$RUN_ID"
```

Keep the entire checkout: GraphQL fragments and CSV data are companion files,
not optional downloads. The load-testing example sends only a short bounded
probe to the local fixture. Provider-specific recipes in the manual require real
provider configuration; these local examples do not claim to verify it.

## Browse and build the site

From `site/`:

```bash
npm ci
npm run dev
```

The dev server prints its URL. For a static build and checks:

```bash
npm run check
npm run lint
npm run build
```

## Keep examples executable

Each `.svx` page declares its complete source with a `cookbook-source` marker and
shows that file verbatim in a YAML block. `TestCookbookRecipes` compares the bytes,
executes every page's Prerequisites and Run it blocks in a temporary checkout
against an ephemeral Mudflat server, and fails if any page is missing its recipe.
Run it with:

```bash
go test ./cmd/curlew -run '^TestCookbookRecipes$' -count=1 -v
```

The normal Go gate runs this test. Timings and fabricated terminal output are not
published as evidence. The “Local example” badge means the scenario has runnable
local fixtures; it does not mean a public website or cloud integration was tested.
