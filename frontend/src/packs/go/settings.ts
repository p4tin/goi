// Goi's settings: gopls, the Go SDK and environment, Delve, golangci-lint
// and the Go formatter. The settings view shows them through
// registerSettings.

import { both, opts, registerSettings, userOnly } from '../../views/settings/schema';
import type { SettingDescriptor } from '../../views/settings/schema';

export const goSettings: SettingDescriptor[] = [
  {
    key: 'gopls.enabled',
    title: 'Enable gopls',
    section: 'Go',
    description: 'Code completion, navigation, diagnostics and refactoring through the Go language server.',
    type: 'bool',
    layers: both,
  },
  {
    key: 'gopls.path',
    title: 'gopls Path',
    section: 'Go',
    description: 'gopls executable. Empty searches $PATH, $GOBIN, $GOPATH/bin and the tools folder.',
    type: 'string',
    layers: userOnly,
  },
  { key: 'gopls.usePlaceholders', title: 'Complete Function Parameters', section: 'Go', description: 'Completing a function inserts placeholders for its parameters.', type: 'bool', layers: both },
  { key: 'gopls.staticcheck', title: 'Staticcheck Analyzers', section: 'Go', description: 'Report the extra staticcheck analyzers in diagnostics (slower on large projects).', type: 'bool', layers: both },
  { key: 'debug.dlvPath', title: 'Delve Path', section: 'Run & Debug', description: "dlv executable. Empty searches $PATH, $GOBIN, $GOPATH/bin and the tools folder.", type: 'string', layers: userOnly },
  { key: 'lint.enabled', title: 'Enable golangci-lint', section: 'Linting', description: 'Show golangci-lint issues in the editor and Problems.', type: 'bool', layers: both },
  { key: 'lint.onSave', title: 'Lint on Save', section: 'Linting', description: "Lint the saved file's package.", type: 'bool', layers: both },
  { key: 'lint.golangciPath', title: 'golangci-lint Path', section: 'Linting', description: "golangci-lint executable. Empty searches $PATH, $GOBIN, $GOPATH/bin and the tools folder.", type: 'string', layers: userOnly },
  { key: 'lint.args', title: 'Extra Arguments', section: 'Linting', description: 'Added to golangci-lint run (the project .golangci.yml is always used).', type: 'stringList', layers: both },
  { key: 'go.goroot', title: 'GOROOT', section: 'Go', description: 'Go SDK used for this project. Empty picks one automatically.', type: 'string', layers: both },
  { key: 'go.env', title: 'Environment Variables', section: 'Go', description: 'Extra environment for go commands (GOPROXY, GOPRIVATE, GOFLAGS…).', type: 'map', layers: both },
  { key: 'go.tags', title: 'Build Tags', section: 'Go', description: 'Build tags passed as -tags to go commands.', type: 'stringList', layers: both },
  { key: 'go.goos', title: 'GOOS', section: 'Go', description: 'Target operating system. Empty uses the host.', type: 'string', layers: both },
  { key: 'go.goarch', title: 'GOARCH', section: 'Go', description: 'Target architecture. Empty uses the host.', type: 'string', layers: both },
  {
    key: 'go.cgo',
    title: 'CGO_ENABLED',
    section: 'Go',
    description: 'Enable or disable cgo. Default uses the toolchain default.',
    type: 'enum',
    options: opts(['', 'Default'], ['1', 'Enabled'], ['0', 'Disabled']),
    layers: both,
  },
  {
    key: 'go.vendor',
    title: 'Vendoring',
    section: 'Go',
    description: 'Auto uses vendor/ when present; On forces -mod=vendor; Off forces -mod=mod.',
    type: 'enum',
    options: opts(['auto', 'Auto'], ['on', 'On'], ['off', 'Off']),
    layers: both,
  },
  {
    key: 'save.format',
    title: 'Format on Save',
    section: 'Save Actions',
    description: 'Formatter run when saving Go files.',
    type: 'enum',
    options: opts(['goimports', 'goimports (format + imports)'], ['gofmt', 'gofmt'], ['none', 'None']),
    layers: both,
  },
];

export function installGoSettings(): void {
  registerSettings({
    sections: [
      { name: 'Go', after: 'Editor' },
      { name: 'Linting', after: 'Version Control' },
    ],
    settings: goSettings,
  });
}
