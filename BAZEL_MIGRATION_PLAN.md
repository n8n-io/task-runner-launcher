# Plan de Migración a Bazel - task-runner-launcher

## 1. Análisis del Proyecto Actual

### Estructura del Proyecto

- **Lenguaje**: Go 1.24.6
- **Arquitectura**: Aplicación CLI con estructura modular
- **Entrada principal**: `cmd/launcher/main.go`
- **Paquetes internos**: 9 módulos en `internal/`
- **Sistema de build actual**: Makefile + Go toolchain
- **Total archivos Go**: 27 (15 archivos fuente + 12 tests)

### Dependencias Externas

```
- github.com/getsentry/sentry-go v0.35.2
- github.com/gorilla/websocket v1.5.3  
- github.com/sethvargo/go-envconfig v1.1.0
- github.com/stretchr/testify v1.8.4
```

### Sistema de Build Actual

```makefile
build: go build -o bin cmd/launcher/main.go
test: go test -race ./...
lint: golangci-lint run
```

### Funcionalidades de Build

- Compilación del binario principal
- Ejecución de tests unitarios con race detection
- Linting con golangci-lint
- Formateo de código
- Generación de coverage reports
- Build multiplataforma (linux/amd64, linux/arm64)

## 2. Objetivos de la Migración a Bazel

### Beneficios Esperados

1. **Build reproducible**: Garantizar builds idénticos en diferentes entornos
2. **Cacheo inteligente**: Acelerar builds incrementales
3. **Paralelización**: Mejorar tiempos de build en sistemas multi-core
4. **Gestión de dependencias**: Control granular sobre dependencias externas con Bzlmod
5. **Integración CI/CD**: Mejor integración con pipelines de deployment
6. **Escalabilidad**: Preparar el proyecto para crecimiento futuro
7. **Módulos modernos**: Aprovechar el sistema Bzlmod para gestión de dependencias más limpia

### Compatibilidad con Flujo Actual

- Mantener compatibilidad con comandos existentes
- Preservar funcionalidad de tests y linting
- Conservar targets de release multiplataforma

## 3. Estructura de Build Propuesta

### Archivos Bazel Principales

#### MODULE.bazel

```starlark
module(
    name = "task_runner_launcher",
    version = "1.0.0",
)

# Bazel dependencies
bazel_dep(name = "rules_go", version = "0.46.0")
bazel_dep(name = "gazelle", version = "0.35.0")

# Go toolchain
go_sdk = use_extension("@rules_go//go:extensions.bzl", "go_sdk")
go_sdk.download(version = "1.24.6")

# Go dependencies
go_deps = use_extension("@gazelle//:extensions.bzl", "go_deps")
go_deps.from_file(go_mod = "//:go.mod")

# Use all dependencies from go.mod
use_repo(
    go_deps,
    "com_github_getsentry_sentry_go",
    "com_github_gorilla_websocket", 
    "com_github_sethvargo_go_envconfig",
    "com_github_stretchr_testify",
    # Indirect dependencies
    "com_github_davecgh_go_spew",
    "com_github_kr_text",
    "com_github_pmezard_go_difflib",
    "org_golang_x_sys",
    "org_golang_x_text",
    "in_gopkg_yaml_v3",
)
```

#### BUILD.bazel (root)

```starlark
load("@gazelle//:def.bzl", "gazelle")

# gazelle:prefix task-runner-launcher
gazelle(name = "gazelle")

gazelle(
    name = "gazelle-update-repos",
    args = [
        "-from_file=go.mod",
        "-to_macro=go_deps.bzl%go_dependencies",
        "-prune",
    ],
    command = "update-repos",
)

# Lint and format targets
sh_binary(
    name = "golangci-lint",
    srcs = ["scripts/golangci-lint.sh"],
)

alias(
    name = "lint",
    actual = ":golangci-lint",
)

sh_binary(
    name = "gofmt",
    srcs = ["scripts/gofmt.sh"],
)

alias(
    name = "fmt",
    actual = ":gofmt",
)

sh_binary(
    name = "gofmt-check", 
    srcs = ["scripts/gofmt-check.sh"],
)

alias(
    name = "fmt-check",
    actual = ":gofmt-check",
)
```

#### cmd/launcher/BUILD.bazel

```starlark
load("@rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "launcher_lib",
    srcs = ["main.go"],
    importpath = "task-runner-launcher/cmd/launcher",
    visibility = ["//visibility:private"],
    deps = [
        "//internal/commands",
        "//internal/config",
        "//internal/errorreporting",
        "//internal/http",
        "//internal/logs",
        "@com_github_sethvargo_go_envconfig//:envconfig",
    ],
)

go_binary(
    name = "launcher",
    embed = [":launcher_lib"],
    visibility = ["//visibility:public"],
)

go_binary(
    name = "task-runner-launcher",
    embed = [":launcher_lib"],
    visibility = ["//visibility:public"],
)

# Cross-compilation targets
go_binary(
    name = "task-runner-launcher-linux-amd64",
    embed = [":launcher_lib"],
    goarch = "amd64",
    goos = "linux",
    visibility = ["//visibility:public"],
)

go_binary(
    name = "task-runner-launcher-linux-arm64", 
    embed = [":launcher_lib"],
    goarch = "arm64",
    goos = "linux",
    visibility = ["//visibility:public"],
)
```

#### internal/commands/BUILD.bazel

```starlark
load("@rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "commands",
    srcs = ["launch.go"],
    importpath = "task-runner-launcher/internal/commands",
    visibility = ["//visibility:public"],
    deps = [
        "//internal/config",
        "//internal/env",
        "//internal/errs",
        "//internal/http",
        "//internal/logs",
        "//internal/ws",
    ],
)

go_test(
    name = "commands_test",
    srcs = ["launch_test.go"],
    embed = [":commands"],
    deps = [
        "@com_github_stretchr_testify//assert",
        "@com_github_stretchr_testify//require",
    ],
)
```

## 4. Plan de Implementación

### Fase 1: Configuración Inicial (Semana 1)

1. **Instalar Bazel 8.x**: Configurar Bazel 8+ en entorno de desarrollo
2. **Crear MODULE.bazel**: Definir módulo y dependencias con Bzlmod
3. **Configurar Go toolchain**: Establecer versión Go 1.24.6
4. **Setup Gazelle**: Configurar generación automática de BUILD files con Bzlmod

### Fase 2: Migración de Targets Básicos (Semana 1-2)

1. **Generar BUILD files**: Usar Gazelle para crear BUILD.bazel iniciales
2. **Target binario principal**: Migrar `cmd/launcher`
3. **Librerías internas**: Configurar todos los paquetes en `internal/`
4. **Validar build básico**: Verificar que `bazel build //cmd/launcher` funciona

### Fase 3: Migración de Tests (Semana 2)

1. **Test targets**: Configurar todos los `go_test` targets
2. **Test con race detection**: Configurar `bazel test --@rules_go//go/config:race //...`
3. **Test coverage**: Implementar generación de coverage reports
4. **Validar tests**: Asegurar que todos los tests pasan

### Fase 4: Herramientas de Desarrollo (Semana 2-3)

1. **Linting**: Integrar golangci-lint via shell scripts
2. **Formateo**: Configurar gofmt checks
3. **Scripts de desarrollo**: Crear wrappers para comandos comunes
4. **Alias targets**: Crear aliases para compatibilidad

### Fase 5: Build Multiplataforma (Semana 3)

1. **Cross-compilation**: Configurar builds para linux/amd64 y linux/arm64
2. **Release targets**: Crear targets para generar binarios de release
3. **Validar releases**: Probar generación de artefactos

### Fase 6: Integración CI/CD (Semana 3-4)

1. **GitHub Actions**: Actualizar workflows para usar Bazel 8+
2. **Cacheo remoto**: Configurar remote caching si es necesario
3. **Performance**: Optimizar builds en CI
4. **Rollback plan**: Mantener Makefile como backup inicial

## 5. Comandos Equivalentes

### Build Commands

```bash
# Makefile actual → Bazel
make build          → bazel build //cmd/launcher:task-runner-launcher
make test           → bazel test //...
make test-verbose   → bazel test //... --test_output=all
make test-coverage  → bazel coverage //...
make lint           → bazel run //:lint
make fmt            → bazel run //:fmt
make fmt-check      → bazel run //:fmt-check
```

### Nuevos Comandos Bazel

```bash
# Builds optimizados
bazel build -c opt //cmd/launcher:task-runner-launcher

# Tests con race detection
bazel test --@rules_go//go/config:race //...

# Build multiplataforma
bazel build //cmd/launcher:task-runner-launcher-linux-amd64
bazel build //cmd/launcher:task-runner-launcher-linux-arm64

# Clean builds
bazel clean --expunge

# Actualizar dependencias Go desde go.mod
bazel run //:gazelle-update-repos
```

## 6. Configuraciones Especiales

### .bazelrc

```bash
# Habilitar Bzlmod
common --enable_bzlmod=true

# Build flags
build --@rules_go//go/config:pure

# Test flags
test --test_output=errors
test --@rules_go//go/config:race

# Optimization flags
build:opt -c opt
build:opt --copt=-O2
build:opt --linkopt=-s

# CI flags
build:ci --verbose_failures
build:ci --test_summary=detailed
test:ci --test_output=all

# Local development
build:dev --disk_cache=~/.cache/bazel-disk-cache
build:dev --repository_cache=~/.cache/bazel-repository-cache
```

### scripts/golangci-lint.sh

```bash
#!/bin/bash
set -euo pipefail

if ! command -v golangci-lint &> /dev/null; then
    echo "golangci-lint not found, installing..."
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
fi

exec golangci-lint run "$@"
```

### scripts/gofmt.sh

```bash
#!/bin/bash
set -euo pipefail

find . -name "*.go" -not -path "./bazel-*" | xargs gofmt -w
```

### scripts/gofmt-check.sh

```bash
#!/bin/bash
set -euo pipefail

unformatted=$(find . -name "*.go" -not -path "./bazel-*" | xargs gofmt -l)
if [ -n "$unformatted" ]; then
    echo "Found unformatted Go files:"
    echo "$unformatted"
    echo "Please run 'bazel run //:fmt'"
    exit 1
fi
```

### Makefile de Transición

```makefile
# Mantener compatibilidad durante migración
.PHONY: bazel-build bazel-test bazel-clean

bazel-build:
	bazel build //cmd/launcher:task-runner-launcher

bazel-test:
	bazel test //...

bazel-clean:
	bazel clean

# Gradualmente reemplazar targets existentes
build: bazel-build
test: bazel-test
clean: bazel-clean
```

## 7. Consideraciones Especiales

### Gestión de Dependencias con Bzlmod

- **go.mod como fuente de verdad**: Mantener go.mod para definir dependencias
- **MODULE.bazel para Bazel**: Usar extensiones go_deps para importar desde go.mod
- **Version pinning automático**: Bzlmod maneja resolución de versiones automáticamente
- **Dependency updates**: Actualizar go.mod y ejecutar `bazel run //:gazelle-update-repos`

### Performance

- **Build cache**: Configurar cache local agresivo con disk_cache
- **Repository cache**: Cachear descargas de dependencias
- **Remote cache**: Evaluar necesidad de cache remoto para equipo
- **Incremental builds**: Bzlmod mejora la eficiencia de builds incrementales

### Compatibilidad

- **Developer experience**: Comandos familiares a través de aliases
- **CI/CD integration**: Workflows actualizados para Bazel 8+
- **Rollback strategy**: Plan para revertir a Makefile si es necesario

## 8. Validación y Testing

### Criterios de Éxito

1. **Functional parity**: Todos los comandos make tienen equivalente Bazel
2. **Performance**: Builds Bazel ≤ tiempo de builds Make (con cache)
3. **CI/CD**: Workflows GitHub Actions funcionan correctamente
4. **Developer adoption**: Desarrolladores pueden usar Bazel día a día
5. **Reliability**: No regresiones en funcionalidad
6. **Bzlmod compatibility**: Aprovecha beneficios del sistema moderno de módulos

### Plan de Testing

1. **Unit tests**: Todos los tests pasan con Bazel
2. **Integration tests**: Build completo + deployment funciona
3. **Performance tests**: Comparar tiempos de build
4. **Regression tests**: Validar no hay cambios en binario final
5. **Dependency resolution**: Verificar resolución correcta con Bzlmod

## 9. Documentación y Training

### Documentación a Actualizar

- **docs/development.md**: Añadir instrucciones Bazel 8+ y Bzlmod
- **README.md**: Actualizar comandos de build
- **CI/CD docs**: Actualizar workflows

### Training Necesario

- **Bazel 8+ basics**: Conceptos fundamentales y Bzlmod
- **Migration timeline**: Comunicar fechas y expectations
- **Support**: Canal para resolver dudas durante migración

## 10. Timeline y Milestones

### Milestone 1 (Semana 1)

- [ ] MODULE.bazel configurado con Bzlmod
- [ ] BUILD files generados con Gazelle
- [ ] Build básico funcionando
- [ ] Tests básicos funcionando

### Milestone 2 (Semana 2)

- [ ] Todos los tests migrados y pasando
- [ ] Linting integrado via shell scripts
- [ ] Coverage reports funcionando
- [ ] Documentación actualizada

### Milestone 3 (Semana 3)

- [ ] Builds multiplataforma funcionando
- [ ] CI/CD actualizado a Bazel 8+
- [ ] Performance validada
- [ ] Team training completado

### Milestone 4 (Semana 4)

- [ ] Migración completa
- [ ] Makefile deprecated/removido
- [ ] Documentación final actualizada
- [ ] Post-migration review

## 11. Riesgos y Mitigaciones

### Riesgos Identificados

1. **Learning curve**: Equipo no familiar con Bazel 8+ y Bzlmod
    - **Mitigación**: Training sessions y documentación detallada sobre Bzlmod

2. **Bzlmod adoption**: Sistema relativamente nuevo puede tener issues
    - **Mitigación**: Testing exhaustivo y plan de rollback a WORKSPACE si es necesario

3. **Performance regression**: Builds más lentos que Make
    - **Mitigación**: Profiling y optimización de configuración

4. **CI/CD issues**: Problemas en deployment pipeline
    - **Mitigación**: Testing exhaustivo en branch separado

5. **Dependency resolution**: Problemas con resolución de dependencias en Bzlmod
    - **Mitigación**: Validación temprana de todas las deps y fallback a go.mod

### Plan de Rollback

- Mantener Makefile funcional durante período de transición
- Branch dedicado para migración Bazel
- Métricas de performance antes/después
- Rollback automático si CI falla por más de 2 días
- Opción de revertir a sistema WORKSPACE si Bzlmod presenta problemas

Este plan de migración actualizado aprovecha las ventajas de Bazel 8+ con Bzlmod, proporcionando una gestión de
dependencias más moderna y eficiente mientras mantiene la compatibilidad con el flujo de trabajo existente.