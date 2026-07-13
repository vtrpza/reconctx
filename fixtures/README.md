# Fixture Workspace

## Boundaries

- `private-captures/` guarda outputs originais e nunca deve entrar em Git.
- `fixtures/cases/` receberá somente cópias sanitizadas e aprovadas.
- `fixtures/shared/arjun-minimal.txt` é a wordlist determinística do target local.
- Cada caso precisa de `manifest.json`, `expected.json`, stdout, stderr, output nativo e checksums.
- `origin=captured` significa output produzido pela tool real.
- `origin=derived` exige `source_case_id` e descrição da transformação.
- Nenhuma fixture externa pode ser capturada sem domínio controlado/autorizado.

## Stop gate

Preparar arquivos não autoriza Katana, GAU ou Arjun. Antes da primeira execução, mostrar ao operador target, argv completo, rate, concurrency, timeout e output directory.
