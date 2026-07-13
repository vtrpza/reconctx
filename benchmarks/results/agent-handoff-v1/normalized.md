# Resultado — condição NORMALIZED

## Proveniência e leitura

- **Único arquivo consultado:** `/tmp/reconctx-benchmark-v1/normalized/records.jsonl`
- **Tamanho medido/leitura integral:** **69.529 bytes**
- **Leituras integrais realizadas:** 2 — uma para leitura e outra para parsing estruturado.
- **Total agregado lido do arquivo:** **139.058 bytes**
- **Registros processados:** 84:
  - 1 `run`
  - 6 `tool_execution`
  - 3 `asset`
  - 11 `endpoint`
  - 7 `parameter`
  - 17 `observation`
  - 15 `evidence`
  - 24 `relationship`
- **Outros arquivos/diretórios consultados:** nenhum.
- **Arquivos criados ou modificados:** nenhum.
- Nenhum scanner foi executado e nenhuma rede foi acessada.

## 1. Assets/origins in-scope: atuais versus apenas históricos

Há **3 origins in-scope**.

### Atual/observado ativamente nesta run

1. **`http://127.0.0.1:18080`**
   - Asset: `asset_sha256_3b75992fdc77bf56ab9b66897aacf38867fd13b66c4e1af3768a9e7eefb11634`
   - Classificação: `in_scope`
   - Katana obteve respostas HTTP nesta run; portanto, “atual” aqui significa **observado ativamente na captura**, não uma garantia de disponibilidade futura.
   - Evidence IDs Katana:
     - `ev_sha256_06927aeeb698c5b07ad88ba075c132b9d90cdf6bb60ebb114d44241bb43e94b9`
     - `ev_sha256_2d8af76e95df4b5d47eaf8ca8068936ca720b50f14968b74f3ab5b7972275a5e`
     - `ev_sha256_568f6c76bbacac05d2052a0c4630f75f8762198b765a35fe02fca35bf4440dc0`
     - `ev_sha256_9d07d34ea8d2d6784dcb84616348ee0d7b3a3c6109c7129b26b402c571f2e07e`
     - `ev_sha256_43e65c683383800879663f14c81e67acd4a1c3a9a17168a74e68178d83bd7c32`
     - `ev_sha256_e9988ee0f327265287baa0c5dcb386b867075b272d57ae87b9cfedab76123ade`

### Apenas históricos

2. **`https://fixture.test`**
   - Asset: `asset_sha256_d0ce6d2552774fc27dd3fdd197717f8f1e1ec4e055e5012a5462c0265af1be09`
   - Única observação: `obs_sha256_a07a17cf0d70e39d65f4074cc31d4e4e2e6cf09f7b47fc2d86c079913de627cc`
   - Estado: `historical`; `current_reachability=unknown`
   - Evidence: `ev_sha256_1a1d4c21323eb6df35cb24dcbfd869048f0193d350e04343b83b97375d407b24`

3. **`https://finance.fixture.test`**
   - Asset: `asset_sha256_0dc7fbbd1680973b04ff1124246a18eed3b14c58d65b561b6f1084f0db06ec6a`
   - Observações:
     - `obs_sha256_db06c1781182196b8f0100abf3088595f4664e0874b1ce682192e000e96cbefe`
     - `obs_sha256_f35a96ef1a91e17cda46e1207252b2a051b9d4faa5de0592e1cdf39c7a2f8df8`
   - Estado: `historical`; `current_reachability=unknown`
   - Evidence:
     - `ev_sha256_8e2e1d31716fd19d9075e1e0e0ffc73baf5fd2078a306a030a3d6b1ed9712056`
     - `ev_sha256_533e1bb263896ce0da5ef570a3b4ee559350034d24ed738851933e4a8ee0abd2`

## 2. Endpoints observados pelo Katana nesta run

O Katana observou **6 rotas**, todas por `GET` e com resposta HTTP `200`:

| URL efetivamente observada | Endpoint/rota normalizada | Observation ID | Evidence ID |
|---|---|---|---|
| `http://127.0.0.1:18080/` | `ep_sha256_66b8b662bd3be18846a585cd79608a45cfc6d923dbcf0b7e8e269c81298e7045` | `obs_sha256_90b4c61d19df2d5986c99c5aa6d85f122b6871f4a3b47f8a3d80d1c80ca35e17` | `ev_sha256_06927aeeb698c5b07ad88ba075c132b9d90cdf6bb60ebb114d44241bb43e94b9` |
| `http://127.0.0.1:18080/static/app.js` | `ep_sha256_1d09c312cd6b38ed17ad7acc621fd89e152d53c21f6ed3f32e0cdabae80e2e0f` | `obs_sha256_6508240e7ce6257bbb01b677e1cc6a65d69900fbc5667f544d741d032e0db18b` | `ev_sha256_2d8af76e95df4b5d47eaf8ca8068936ca720b50f14968b74f3ab5b7972275a5e` |
| `http://127.0.0.1:18080/api/search` | `ep_sha256_6243f00179cd009c1f49b6e6bcf45f1aeabde50508c157526333a1fb12acfb0a` | `obs_sha256_5ef46794f433e15c6df71bd1db5c1e5676bc221214d0dd82b795f4473737c3ec` | `ev_sha256_568f6c76bbacac05d2052a0c4630f75f8762198b765a35fe02fca35bf4440dc0` |
| `http://127.0.0.1:18080/api/users?id=1` | `ep_sha256_6a00d1982db0e61a0b66d781d1ff2143b18d66aa960e059a542beb89e55ebfde` (`/api/users`) | `obs_sha256_6d338db24d8531457fc55c03b4d477fcde591598ba589495de5280a9fd50e624` | `ev_sha256_9d07d34ea8d2d6784dcb84616348ee0d7b3a3c6109c7129b26b402c571f2e07e` |
| `http://127.0.0.1:18080/search?q=seed` | `ep_sha256_6bfdd628024a77379fa434b7665b0ecaebe5b12511fd36940bdeff3e90f9d7dc` (`/search`) | `obs_sha256_dc740fafbb270a3902c8b073b158ebd7441f4d267fed571474a0c878716f1b95` | `ev_sha256_43e65c683383800879663f14c81e67acd4a1c3a9a17168a74e68178d83bd7c32` |
| `http://127.0.0.1:18080/login` | `ep_sha256_503db1a9f9aa627f008a5fe0c02737e4f59572f813218fe995fe73de8f0fe2ae` | `obs_sha256_7cb4904fe719ef6140fb658cf1b378597dbc15367153acd9ef10ef190908e4b3` | `ev_sha256_e9988ee0f327265287baa0c5dcb386b867075b272d57ae87b9cfedab76123ade` |

Execução: `tx_fixture_katana`, status `success`, coverage `complete`.

## 3. URLs provenientes apenas de fontes históricas

Há **2 URLs únicas**, representadas por **3 observações históricas**:

1. **`https://fixture.test/article/`**
   - Endpoint: `ep_sha256_c6f2339c16ed37fa7a7112e83a57333b3cef33ada177b52973f0f58ef8f80ea8`
   - Observation: `obs_sha256_a07a17cf0d70e39d65f4074cc31d4e4e2e6cf09f7b47fc2d86c079913de627cc`
   - Evidence: `ev_sha256_1a1d4c21323eb6df35cb24dcbfd869048f0193d350e04343b83b97375d407b24`

2. **`https://finance.fixture.test/`**
   - Endpoint: `ep_sha256_561aa7d170744d12ce0b49337fc2d9539761f3b91f56181df037268fc5ba1923`
   - Duas ocorrências históricas:
     - `obs_sha256_db06c1781182196b8f0100abf3088595f4664e0874b1ce682192e000e96cbefe` → `ev_sha256_8e2e1d31716fd19d9075e1e0e0ffc73baf5fd2078a306a030a3d6b1ed9712056`
     - `obs_sha256_f35a96ef1a91e17cda46e1207252b2a051b9d4faa5de0592e1cdf39c7a2f8df8` → `ev_sha256_533e1bb263896ce0da5ef570a3b4ee559350034d24ed738851933e4a8ee0abd2`

Todas vieram de `tx_fixture_gau`. A disponibilidade atual permanece desconhecida.

## 4. Entidades com evidência em mais de uma source family

Interpretação conservadora: **source family = família da ferramenta** (`gau`, `katana` ou `arjun`), agrupando todas as execuções/modos do Arjun.

Foram encontradas **2 entidades normalizadas** com evidência de mais de uma família:

1. **Origin `http://127.0.0.1:18080`**
   - Asset: `asset_sha256_3b75992fdc77bf56ab9b66897aacf38867fd13b66c4e1af3768a9e7eefb11634`
   - Famílias: Katana + Arjun
   - Evidência representativa:
     - Katana: `ev_sha256_06927aeeb698c5b07ad88ba075c132b9d90cdf6bb60ebb114d44241bb43e94b9`
     - Arjun: `ev_sha256_75efb5db182e91af44bfd8135c1984d57c0da5d98e78bc40d63a5ccf502bee6b`

2. **Endpoint `GET http://127.0.0.1:18080/api/search`**
   - Endpoint: `ep_sha256_6243f00179cd009c1f49b6e6bcf45f1aeabde50508c157526333a1fb12acfb0a`
   - Famílias: Katana + Arjun
   - Evidence:
     - Katana: `ev_sha256_568f6c76bbacac05d2052a0c4630f75f8762198b765a35fe02fca35bf4440dc0`
     - Arjun: `ev_sha256_75efb5db182e91af44bfd8135c1984d57c0da5d98e78bc40d63a5ccf502bee6b`

Os dois providers declarados pelo GAU, `otx` e `urlscan`, **não foram contados como famílias independentes por URL**, porque o próprio registro informa que não há atribuição por linha.

## 5. Parâmetros descobertos pelo Arjun

O Arjun encontrou **5 parâmetros**:

| Endpoint e método | Parâmetro | Location | Parameter/Observation ID | Evidence ID |
|---|---|---|---|---|
| `GET http://127.0.0.1:18080/api/search` | `q` | `query` | `param_sha256_bb98e3aa3ac9685d9c7e6e05b9ef790be4c1debce5074cb319d63bdf1e6dd8db` / `obs_sha256_96c399c356d2bce638f6f14d1d0c1cc081119e024c1011d1605e5acca002bf0b` | `ev_sha256_75efb5db182e91af44bfd8135c1984d57c0da5d98e78bc40d63a5ccf502bee6b` |
| `POST http://127.0.0.1:18080/api/update` | `id` | `form` | `param_sha256_c6f749f77e1cebf04006b257a264d9f46ed0a9facd95a43c61fa3ac0f44be12c` / `obs_sha256_d276cdfe9459432a438cc73948d9165cdec86e5fbbe81f2bf398af0eb7a39ed6` | `ev_sha256_4abfab1554d65e82445e62b6eeae812cc38a12609d7c6e44e2d4eaa7659d0bc6` |
| `POST http://127.0.0.1:18080/api/update` | `name` | `form` | `param_sha256_9f9e8b965041bcb2cbc43d6ad6aad3b2fc634984649f06a7949c1100236bec9e` / `obs_sha256_eeb228717e5de83fed3ef3ded3a866d1acb5297edefdad7b70cb265c3415c07a` | `ev_sha256_88a60c86a81b055e8900abad30e0e50ffe2ae30f573861589d84698537a01e0b` |
| `POST http://127.0.0.1:18080/api/json` em modo JSON | `filter` | `json` | `param_sha256_4338807c222e0be454a586ce69541d3d43eac5513df0e7592802de51b3077619` / `obs_sha256_46853918011413d0dbdb0ad3ff52d43ab6ceb2c4a7f87e408262d817c62748df` | `ev_sha256_115ea733e15d5ede602985dfd591edc56f64e52a69ff08e45bf3248dcb5c16b1` |
| `POST http://127.0.0.1:18080/api/json` em modo JSON | `id` | `json` | `param_sha256_3ca64bb188188ba2966267a88015d264b12a4edb99970397b6e9c4c113c5f28a` / `obs_sha256_e8d7df67d84df25671f3d4bfccbfbb2ad7b657071be74639e42db97901e715f9` | `ev_sha256_a6cfc76ba3d87549fe4699096c425ec5ed3ff1764eaa9269e019481ff22f1543` |

Para todos os cinco, `acceptance_state=unknown` e `detection_basis=null`. Logo, descoberta não equivale a aceitação funcional ou vulnerabilidade.

## 6. Falhas, resultados parciais e coverage gaps

### Falhas

- **Nenhuma execução possui status `failed` ou exit code diferente de zero.**
- Também não há status explícito `partial`.

### Gaps e limitações

1. **Arjun GET**
   - Execução: `tx_fixture_arjun_get`
   - Status: `success`, coverage `complete`
   - Gap: `arjun.fixture_false_negative`
   - Parâmetro conhecido mas não detectado: `debug`
   - **Evidence ID do gap:** nenhum; o campo `evidence_ids` do gap está vazio.

2. **Arjun JSON**
   - Execução: `tx_fixture_arjun_json`
   - Status: `success`, coverage `complete`
   - Mesmo gap/false negative para `debug`
   - **Evidence ID do gap:** nenhum; `evidence_ids=[]`.

3. **Arjun zero-result**
   - Execução: `tx_fixture_arjun_zero`
   - Status: `success_zero`
   - Coverage: `zero`
   - Endpoint: `GET /api/no-params`, `ep_sha256_9fa62c04e78c4a79038881bc6f884054571a81973de217e7dd39026326ffa771`
   - Native output marcado como ausente; stdout registra “No parameters were discovered.”
   - Observation: `obs_sha256_3a6e2c51ba36cf31aeef450fc397ee2469f948648afa2d375dfd32c56ac1268d`
   - Evidence: `ev_sha256_3097141d12d628af911b4efb363a7f18ac6a0e7615e9be1ec81cf42bf0a58d6c`
   - Isso é zero-result, **não falha da ferramenta** e tampouco prova inexistência de parâmetros.

4. **GAU**
   - Execução: `tx_fixture_gau`
   - Status: `success`, coverage `complete`
   - Warning: `gau.provider_attribution_run_level`
   - O conjunto `{otx, urlscan}` é conhecido, mas não há atribuição de provider por URL/linha.
   - **Evidence ID específico do warning:** nenhum; `evidence_ids=[]`.

5. **Gap global da run**
   - Run: `run_fixture_web_blackbox_v0`
   - Gap: `run.failure_paths_pending`
   - Timeouts, interrupções e malformed output não estão incluídos.
   - **Evidence ID:** nenhum; `evidence_ids=[]`.

6. **Sem gaps declarados**
   - `tx_fixture_katana`
   - `tx_fixture_arjun_post_form`

## 7. Candidatos excluídos do Arjun

**Desconhecido / não determinável.**

O arquivo permitido não contém:

- record type de queue/candidate/exclusion;
- artefato de fila;
- decisões de inclusão/exclusão;
- razões de exclusão.

Portanto, **nenhum candidato excluído pode ser nomeado ou inferido**. Em particular, não é válido assumir que endpoints observados pelo Katana mas não testados pelo Arjun foram “excluídos”.

- Record ID: nenhum aplicável.
- Evidence ID: nenhum aplicável.
- Fundamento: ausência desses tipos de registro/artefato no inventário completo do único arquivo permitido.

## 8. Mapeamento de Evidence IDs

Os Evidence IDs estão associados diretamente em cada uma das respostas 1–7. Para as respostas baseadas em lacunas metadadas ou ausência de registros:

- gaps `debug`: sem Evidence ID anexado;
- warning de atribuição GAU: sem Evidence ID anexado;
- gap global de failure paths: sem Evidence ID anexado;
- ausência de queue/exclusions: não existe Evidence ID capaz de provar uma ausência;
- ausência de vulnerabilidade confirmada: igualmente não possui Evidence ID positivo.

Nesses casos, a sustentação disponível é o respectivo **record ID** ou o inventário completo do arquivo, não um Evidence ID.

## 9. Lacunas que devem orientar a próxima execução

1. **Reexecutar/expandir Arjun para `debug`**
   - Alvos indicados pelos gaps:
     - `GET /api/search` — `tx_fixture_arjun_get`
     - JSON/`POST /api/json` — `tx_fixture_arjun_json`
   - O gap não possui Evidence ID próprio.

2. **Registrar a fila de candidatos**
   - Emitir queue artifact contendo candidato, decisão, razão e relação com endpoints de entrada.
   - Necessário para responder de forma verificável quais endpoints foram excluídos.

3. **Verificar reachability das URLs históricas**
   - `https://fixture.test/article/`
   - `https://finance.fixture.test/`
   - Atualmente ambas possuem `current_reachability=unknown`.
   - Evidence: `ev_sha256_1a1d4c21323eb6df35cb24dcbfd869048f0193d350e04343b83b97375d407b24`, `ev_sha256_8e2e1d31716fd19d9075e1e0e0ffc73baf5fd2078a306a030a3d6b1ed9712056`, `ev_sha256_533e1bb263896ce0da5ef570a3b4ee559350034d24ed738851933e4a8ee0abd2`.

4. **Obter atribuição histórica por provider**
   - Preservar provider por linha/URL para distinguir OTX de URLScan.
   - Gap em `tx_fixture_gau`; sem Evidence ID específico.

5. **Validar os parâmetros encontrados**
   - Todos os resultados Arjun têm `acceptance_state=unknown`.
   - A próxima execução deve confirmar aceitação, efeito comportamental e possíveis diferenças de resposta, sem tratar descoberta como vulnerabilidade.

6. **Reavaliar o zero-result**
   - `/api/no-params` teve coverage `zero`; ampliar wordlist/métodos se esse endpoint continuar relevante.
   - Evidence: `ev_sha256_3097141d12d628af911b4efb363a7f18ac6a0e7615e9be1ec81cf42bf0a58d6c`.

7. **Explicitar cobertura dos demais endpoints Katana**
   - Sem queue artifact, não se sabe por que `/api/users`, `/search`, `/login` e outros não aparecem como alvos Arjun.
   - Isso é uma lacuna de rastreabilidade, não evidência de exclusão.

8. **Cobrir caminhos de falha**
   - Adicionar casos de timeout, interrupção e malformed output conforme `run.failure_paths_pending`.

## 10. Vulnerabilidade confirmada

**Não há vulnerabilidade confirmada no arquivo.**

- Não há records dos tipos `finding` ou `vulnerability`.
- Os parâmetros Arjun foram somente `bruteforced`, todos com `acceptance_state=unknown`.
- Os parâmetros observados pelo Katana (`q` e `id`) apenas aparecem em URLs solicitadas.
- Respostas HTTP `200`, descoberta de endpoint ou descoberta de parâmetro não constituem, isoladamente, vulnerabilidade.

**Evidence ID de “nenhuma vulnerabilidade”:** nenhum — trata-se de ausência de finding no corpus permitido, não de uma evidência positiva de segurança. Isso também **não prova que os alvos estejam livres de vulnerabilidades**.

## Desconhecidos explícitos

- Disponibilidade atual dos dois origins históricos.
- Provider histórico exato por URL.
- Aceitação e impacto dos parâmetros Arjun.
- Candidatos excluídos e respectivas razões.
- Motivo de seleção dos quatro alvos Arjun.
- Comportamento sob timeout, interrupção ou saída malformada.
- Completude do corpus além do único arquivo permitido.
- Existência de vulnerabilidades fora do conteúdo normalizado.

## Autoauditoria de claims não suportados

- **“Atual”** foi inferido conservadoramente de respostas HTTP Katana observadas nesta run; não existe um campo de status atual no asset.
- **“Apenas histórico”** significa apenas que não há observação ativa correspondente no arquivo lido; não afirma indisponibilidade no mundo real.
- **Source family** foi interpretada como família da ferramenta. Atribuição individual OTX/URLScan é desconhecida e não foi inventada.
- **Nenhuma exclusão Arjun foi alegada**, pois não existe queue artifact.
- **Nenhuma vulnerabilidade confirmada** significa “nenhuma registrada neste arquivo”, não “sistema seguro”.
- As recomendações da próxima execução são conclusões operacionais derivadas das lacunas; não são novos fatos observados.