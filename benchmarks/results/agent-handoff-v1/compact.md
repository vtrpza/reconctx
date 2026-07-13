# Resultado — condição COMPACT

## Proveniência e limites da leitura

- **Único arquivo lido:** `/tmp/reconctx-benchmark-v1/compact/CONTEXT.md`
- **Bytes exatos:** `7064 bytes`, medidos diretamente com `wc -c`
- **Extensão lida:** arquivo completo, `CONTEXT.md:L1-L76`
- **Outros arquivos/diretórios inspecionados:** nenhum
- **Drill-down nos locators `raw/...`:** não realizado; os locators abaixo foram apenas transcritos do `CONTEXT.md`
- **Arquivos criados ou modificados:** nenhum
- **Premissa explícita:** o arquivo não contém uma lista separada denominada “in-scope”; portanto, trato os **3 origins do Surface** (`CONTEXT.md:L6,L18-L32`) como os assets/origins solicitados.

---

## 1. Assets/origins atuais versus apenas históricos

O contexto contabiliza **3 origins** (`CONTEXT.md:L6`):

| Classificação | Origin | Fundamentação |
|---|---|---|
| Observado nesta fixture run | `http://127.0.0.1:18080` | Houve respostas HTTP 200 observadas pelo Katana; por exemplo, raiz em **[E01]** → `raw/katana/native-output.jsonl:L1` |
| Apenas histórico | `https://finance.fixture.test` | URL histórica com 2 ocorrências: **[E08]** → `raw/gau/native-output.txt:L3`; **[E12]** → `raw/gau/native-output.txt:L2` |
| Apenas histórico | `https://fixture.test` | URL histórica em **[E03]** → `raw/gau/native-output.txt:L1` |

**Contagem:** 1 origin observado durante a run e 2 origins apenas históricos.

“Atual” significa somente **observado durante esta fixture run**, não que esteja acessível agora. `historical_only` não estabelece alcançabilidade atual (`CONTEXT.md:L7`).

---

## 2. Endpoints observados pelo Katana nesta run

Foram observados **6 endpoints GET**, todos com estado `observed_http 200`:

| Endpoint | Evidence ID e locator |
|---|---|
| `GET http://127.0.0.1:18080/` | **[E01]** → `raw/katana/native-output.jsonl:L1` |
| `GET http://127.0.0.1:18080/static/app.js` | **[E04]** → `raw/katana/native-output.jsonl:L2` |
| `GET http://127.0.0.1:18080/api/search` | **[E09]** → `raw/katana/native-output.jsonl:L3` |
| `GET http://127.0.0.1:18080/api/users` | **[E13]** → `raw/katana/native-output.jsonl:L4` |
| `GET http://127.0.0.1:18080/search` | **[E06]** → `raw/katana/native-output.jsonl:L5` |
| `GET http://127.0.0.1:18080/login` | **[E15]** → `raw/katana/native-output.jsonl:L6` |

Base consolidada: `CONTEXT.md:L22,L25,L27-L30`.

---

## 3. URLs provenientes apenas de fontes históricas

Há **2 URLs canônicas únicas**, correspondendo a **3 ocorrências GAU**:

| URL histórica | Ocorrências | Evidence ID e locator |
|---|---:|---|
| `https://fixture.test/article/` | 1 | **[E03]** → `raw/gau/native-output.txt:L1` |
| `https://finance.fixture.test/` | 2 | **[E12]** → `raw/gau/native-output.txt:L2`; **[E08]** → `raw/gau/native-output.txt:L3` |

Base: `CONTEXT.md:L31-L32`. Não é possível atribuir cada linha a um provider GAU específico (`CONTEXT.md:L51`).

---

## 4. Entidades com evidência em mais de uma source family

O contexto identifica exatamente duas entidades multi-source (`CONTEXT.md:L44-L47`):

1. **Origin `http://127.0.0.1:18080`**
   - Source families: **Arjun e Katana**
   - Evidência representativa:
     - Katana: **[E01]** → `raw/katana/native-output.jsonl:L1`
     - Arjun: **[E02]** → `raw/arjun-json/native-output.json#/http:~1~1127.0.0.1:18080~1api~1json/params/0`
   - O contexto associa ainda: **[E04], [E05], [E06], [E07], [E09], [E10], [E11], [E13], [E14], [E15]**.

2. **Endpoint `GET http://127.0.0.1:18080/api/search`**
   - Source families: **Arjun e Katana**
   - Katana: **[E09]** → `raw/katana/native-output.jsonl:L3`
   - Arjun: **[E10]** → `raw/arjun-get/native-output.json#/http:~1~1127.0.0.1:18080~1api~1search/params/0`

Não há outra entidade explicitamente classificada como multi-source no `CONTEXT.md`.

---

## 5. Parâmetros descobertos pelo Arjun

O Arjun produziu **5 candidatos de parâmetro**:

| Endpoint | Parâmetro | Location | Método/modo | Evidence ID e locator |
|---|---|---|---|---|
| `/api/json` | `filter` | JSON body | `POST`, modo JSON | **[E02]** → `raw/arjun-json/native-output.json#/http:~1~1127.0.0.1:18080~1api~1json/params/0` |
| `/api/json` | `id` | JSON body | `POST`, modo JSON | **[E14]** → `raw/arjun-json/native-output.json#/http:~1~1127.0.0.1:18080~1api~1json/params/1` |
| `/api/search` | `q` | Query string | `GET`, modo GET | **[E10]** → `raw/arjun-get/native-output.json#/http:~1~1127.0.0.1:18080~1api~1search/params/0` |
| `/api/update` | `id` | Form body | `POST`, modo POST | **[E07]** → `raw/arjun-post-form/native-output.json#/http:~1~1127.0.0.1:18080~1api~1update/params/0` |
| `/api/update` | `name` | Form body | `POST`, modo POST | **[E11]** → `raw/arjun-post-form/native-output.json#/http:~1~1127.0.0.1:18080~1api~1update/params/1` |

Base consolidada: `CONTEXT.md:L34-L42`.

Esses resultados são **candidatos**; não provam cobertura completa nem semântica de aceitação pelo servidor (`CONTEXT.md:L36`).

---

## 6. Tools que falharam, foram parciais ou tiveram coverage gaps

### Falhas ou execuções parciais

- **Nenhuma tool aparece como `failed` ou `partial`.**
- Cinco runs constam como `success/complete`:
  - `tx_fixture_arjun_get`
  - `tx_fixture_arjun_json`
  - `tx_fixture_arjun_post_form`
  - `tx_fixture_gau`
  - `tx_fixture_katana`
- `tx_fixture_arjun_zero` terminou como **`success_zero/zero`, exit 0`**, não como falha.

Base: `CONTEXT.md:L9-L16`. O zero bounded tem **[E05]** → `raw/arjun-zero/stdout.raw:L15`.

### Coverage gaps documentados

| Tool/run | Gap |
|---|---|
| `tx_fixture_arjun_get` | Não detectou o parâmetro de ground truth `debug` |
| `tx_fixture_arjun_json` | Não detectou o parâmetro de ground truth `debug` |
| `tx_fixture_gau` | O provider set é conhecido apenas em nível de run; as linhas individuais não têm campo de provider |
| `tx_fixture_arjun_zero` | O zero em `/api/no-params` é limitado ao probe efetuado e não prova ausência universal de parâmetros |

Base: `CONTEXT.md:L11-L15,L51-L53`.

**Limite de evidência:** o arquivo não atribui Evidence IDs específicos às declarações run-level sobre `debug` ou provider attribution. Elas são sustentadas apenas pelos locators internos `CONTEXT.md:L11-L15,L51-L53`; **[E05]** sustenta somente o resultado zero observado.

---

## 7. Candidatos excluídos do Arjun e motivos

**Desconhecido.** Não é possível listar candidatos excluídos nem seus motivos.

O artefato de candidate queue/exclusion está ausente porque o composite foi construído a partir de capturas diretas (`CONTEXT.md:L54`). Isso **não prova que nenhum candidato tenha sido excluído**; prova apenas que a informação necessária não está disponível neste handoff compacto.

- **Evidence ID:** nenhum foi atribuído a essa ausência.
- **Locator disponível:** `CONTEXT.md:L54`.

---

## 8. Cruzamento das respostas com Evidence IDs

| Questão | Evidence IDs |
|---|---|
| Q1 — origins atuais/históricos | **[E01], [E03], [E08], [E12]** |
| Q2 — endpoints Katana | **[E01], [E04], [E09], [E13], [E06], [E15]** |
| Q3 — URLs históricas | **[E03], [E08], [E12]** |
| Q4 — multi-source | **[E01], [E02], [E09], [E10]**; o origin também agrega **[E04]-[E07], [E11], [E13]-[E15]** conforme `CONTEXT.md:L46` |
| Q5 — parâmetros Arjun | **[E02], [E14], [E10], [E07], [E11]** |
| Q6 — status/gaps | **[E05]** somente para o bounded zero; os demais gaps não possuem Evidence ID próprio |
| Q7 — exclusões Arjun | Nenhum Evidence ID; artefato ausente |
| Q9 — próximas lacunas | **[E05]** para o zero; **[E02], [E14], [E10], [E07], [E11]** para candidatos cuja semântica ainda requer validação; demais lacunas são run-level sem Evidence ID |
| Q10 — vulnerabilidade confirmada | Nenhum Evidence ID; declaração explícita em `CONTEXT.md:L56` |

### IDs exatos e locators usados

| Ref. | Evidence ID exato | Locator |
|---|---|---|
| E01 | `ev_sha256_06927aeeb698c5b07ad88ba075c132b9d90cdf6bb60ebb114d44241bb43e94b9` | `raw/katana/native-output.jsonl:L1` |
| E02 | `ev_sha256_115ea733e15d5ede602985dfd591edc56f64e52a69ff08e45bf3248dcb5c16b1` | `raw/arjun-json/native-output.json#/http:~1~1127.0.0.1:18080~1api~1json/params/0` |
| E03 | `ev_sha256_1a1d4c21323eb6df35cb24dcbfd869048f0193d350e04343b83b97375d407b24` | `raw/gau/native-output.txt:L1` |
| E04 | `ev_sha256_2d8af76e95df4b5d47eaf8ca8068936ca720b50f14968b74f3ab5b7972275a5e` | `raw/katana/native-output.jsonl:L2` |
| E05 | `ev_sha256_3097141d12d628af911b4efb363a7f18ac6a0e7615e9be1ec81cf42bf0a58d6c` | `raw/arjun-zero/stdout.raw:L15` |
| E06 | `ev_sha256_43e65c683383800879663f14c81e67acd4a1c3a9a17168a74e68178d83bd7c32` | `raw/katana/native-output.jsonl:L5` |
| E07 | `ev_sha256_4abfab1554d65e82445e62b6eeae812cc38a12609d7c6e44e2d4eaa7659d0bc6` | `raw/arjun-post-form/native-output.json#/http:~1~1127.0.0.1:18080~1api~1update/params/0` |
| E08 | `ev_sha256_533e1bb263896ce0da5ef570a3b4ee559350034d24ed738851933e4a8ee0abd2` | `raw/gau/native-output.txt:L3` |
| E09 | `ev_sha256_568f6c76bbacac05d2052a0c4630f75f8762198b765a35fe02fca35bf4440dc0` | `raw/katana/native-output.jsonl:L3` |
| E10 | `ev_sha256_75efb5db182e91af44bfd8135c1984d57c0da5d98e78bc40d63a5ccf502bee6b` | `raw/arjun-get/native-output.json#/http:~1~1127.0.0.1:18080~1api~1search/params/0` |
| E11 | `ev_sha256_88a60c86a81b055e8900abad30e0e50ffe2ae30f573861589d84698537a01e0b` | `raw/arjun-post-form/native-output.json#/http:~1~1127.0.0.1:18080~1api~1update/params/1` |
| E12 | `ev_sha256_8e2e1d31716fd19d9075e1e0e0ffc73baf5fd2078a306a030a3d6b1ed9712056` | `raw/gau/native-output.txt:L2` |
| E13 | `ev_sha256_9d07d34ea8d2d6784dcb84616348ee0d7b3a3c6109c7129b26b402c571f2e07e` | `raw/katana/native-output.jsonl:L4` |
| E14 | `ev_sha256_a6cfc76ba3d87549fe4699096c425ec5ed3ff1764eaa9269e019481ff22f1543` | `raw/arjun-json/native-output.json#/http:~1~1127.0.0.1:18080~1api~1json/params/1` |
| E15 | `ev_sha256_e9988ee0f327265287baa0c5dcb386b867075b272d57ae87b9cfedab76123ade` | `raw/katana/native-output.jsonl:L6` |

---

## 9. Lacunas que devem orientar a próxima execução

Em ordem prática:

1. **Ampliar/repetir a cobertura Arjun GET e JSON**, verificando especificamente o `debug` não detectado (`CONTEXT.md:L52`).
2. **Não tratar `/api/no-params` como definitivamente sem parâmetros**; variar wordlists, métodos e condições do probe. Evidência bounded: **[E05]**.
3. **Validar a aceitação e a semântica dos candidatos Arjun**, especialmente `/api/json`, `/api/search` e `/api/update`; a descoberta não comprova processamento real (`CONTEXT.md:L36`).
4. **Preservar atribuição GAU por URL/provider**, pois hoje ela existe somente no nível da run (`CONTEXT.md:L51`).
5. **Verificar separadamente a alcançabilidade atual das URLs históricas**, sem inferi-la a partir do GAU (`CONTEXT.md:L7`).
6. **Gerar e guardar a candidate queue/exclusion artifact**, permitindo identificar candidatos removidos e as respectivas justificativas (`CONTEXT.md:L54`).
7. **Capturar fixtures reais de timeout, interrupção e malformed output**, atualmente pendentes (`CONTEXT.md:L55`).
8. **Exercitar contexto autenticado**, que não foi coberto (`CONTEXT.md:L56`).

As ações acima são recomendações derivadas das lacunas documentadas, não observações adicionais sobre o alvo.

---

## 10. Vulnerabilidade confirmada

**Não. Nenhuma vulnerabilidade foi confirmada.**

O próprio handoff afirma isso em `CONTEXT.md:L56`. Os endpoints, parâmetros candidatos e URLs históricas são resultados de reconhecimento, não demonstrações de impacto ou explorabilidade.

- **Evidence ID específico:** nenhum atribuído à declaração.
- **Locator:** `CONTEXT.md:L56`.

---

## Autoverificação de alegações não suportadas

Não afirmo que:

- os origins/URLs históricos estejam alcançáveis atualmente;
- o origin local continue acessível após a fixture run;
- cada URL GAU pertença a um provider histórico específico;
- a lista de parâmetros esteja completa;
- o parâmetro `debug` tenha sido encontrado;
- `/api/no-params` seja universalmente livre de parâmetros;
- os candidatos Arjun sejam aceitos ou processados semanticamente;
- nenhum candidato tenha sido excluído — a informação é desconhecida;
- as tools tenham sido testadas sob timeout, interrupção ou output malformado;
- exista cobertura autenticada;
- exista qualquer vulnerabilidade confirmada;
- os locators `raw/...` tenham sido verificados diretamente.

**Conclusão operacional:** 3 origins, 1 observado na run e 2 apenas históricos; 6 endpoints Katana; 5 candidatos de parâmetro Arjun; nenhuma falha de tool registrada, mas há lacunas claras de cobertura e proveniência. Nenhum arquivo foi criado ou modificado.