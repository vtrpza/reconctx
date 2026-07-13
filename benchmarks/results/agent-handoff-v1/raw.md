# Resultado — condição RAW

## Critério de leitura

- Foram usados **somente** arquivos em `/tmp/reconctx-benchmark-v1/raw`.
- Nenhum scanner foi executado e nenhuma rede foi acessada.
- Nenhum diretório pai/irmão, dado normalizado, `CONTEXT`, schema, teste, manifesto, código-fonte ou benchmark anterior foi inspecionado.
- “Histórico” abaixo significa **proveniência da family GAU/OTX/urlscan**, conforme a categorização pedida. O RAW não traz timestamp de arquivo por URL nem prova que essas URLs estejam indisponíveis hoje.
- `stdout` e `native-output` da mesma tool/run não foram tratados como source families independentes.

---

## 1. Assets/origins e classificação atual versus histórica

### Contagem observável

Há **3 origins distintos nos dados RAW**:

| Origin | Classificação nesta run | Base |
|---|---|---|
| `http://127.0.0.1:18080` | **Atual/observado ativamente** | Katana fez requisições e recebeu HTTP 200; Arjun também escaneou endpoints nesse origin. [/tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl:1-6 — `/request/endpoint`, `/response/status_code`] |
| `https://fixture.test` | **Apenas histórico no RAW** | O GAU produziu `https://fixture.test/article/`; não aparece nos resultados Katana/Arjun. [/tmp/reconctx-benchmark-v1/raw/gau/native-output.txt:1] |
| `https://finance.fixture.test` | **Apenas histórico no RAW** | O GAU produziu a URL duas vezes; não aparece nos resultados Katana/Arjun. [/tmp/reconctx-benchmark-v1/raw/gau/native-output.txt:2-3] |

O log do GAU registra consultas a `fixture.test` nos providers `otx` e `urlscan`. [/tmp/reconctx-benchmark-v1/raw/gau/stderr.raw:2-3]

### Ressalva de escopo

O número de origins **comprovadamente “in-scope” é desconhecido**: nenhum dos arquivos RAW contém uma declaração ou snapshot de escopo. Assim:

- Origins que **aparecem**: **3**.
- Origins atuais/ativos nesta run: **1**.
- Origins somente na family histórica: **2**.
- Origins comprovadamente in-scope: **indisponível no RAW**.

Também não há evidência de que `127.0.0.1:18080` seja uma representação local de `fixture.test`; não fiz essa associação.

---

## 2. Endpoints observados pelo Katana nesta run

O Katana registrou **6 endpoints únicos**, todos com método `GET` e resposta HTTP `200`:

| Endpoint exato | Status | Citação |
|---|---:|---|
| `http://127.0.0.1:18080/` | 200 | [/tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl:1 — `/request/method`, `/request/endpoint`, `/response/status_code`] |
| `http://127.0.0.1:18080/static/app.js` | 200 | [/tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl:2 — `/request/endpoint`, `/response/status_code`] |
| `http://127.0.0.1:18080/api/search` | 200 | [/tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl:3 — `/request/endpoint`, `/response/status_code`] |
| `http://127.0.0.1:18080/api/users?id=1` | 200 | [/tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl:4 — `/request/endpoint`, `/response/status_code`] |
| `http://127.0.0.1:18080/search?q=seed` | 200 | [/tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl:5 — `/request/endpoint`, `/response/status_code`] |
| `http://127.0.0.1:18080/login` | 200 | [/tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl:6 — `/request/endpoint`, `/response/status_code`] |

---

## 3. URLs provenientes apenas de fontes históricas

Há **2 URLs históricas únicas**, em 3 linhas de saída:

1. `https://fixture.test/article/`  
   [/tmp/reconctx-benchmark-v1/raw/gau/native-output.txt:1]

2. `https://finance.fixture.test/`  
   Aparece duplicada. [/tmp/reconctx-benchmark-v1/raw/gau/native-output.txt:2-3]

“Somente históricas” significa que elas aparecem no resultado GAU e não em nenhum resultado Katana/Arjun lido. Isso **não prova** que estejam offline ou que nunca tenham sido observadas ativamente fora deste conjunto RAW. O arquivo também não atribui cada URL individualmente a OTX ou urlscan.

---

## 4. Entidades com evidência em mais de uma source family

Considerando as source families **Katana**, **Arjun** e **GAU**:

### Sobreposição estrita

1. **Origin `http://127.0.0.1:18080`**
   - Katana: seis requisições com resposta. [/tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl:1-6]
   - Arjun: scans de `/api/search`, `/api/update`, `/api/json` e `/api/no-params`.  
     [/tmp/reconctx-benchmark-v1/raw/arjun-get/stdout.raw:6]  
     [/tmp/reconctx-benchmark-v1/raw/arjun-post-form/stdout.raw:6]  
     [/tmp/reconctx-benchmark-v1/raw/arjun-json/stdout.raw:6]  
     [/tmp/reconctx-benchmark-v1/raw/arjun-zero/stdout.raw:6]

2. **Endpoint exato `http://127.0.0.1:18080/api/search`**
   - Katana: observado com GET/200. [/tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl:3]
   - Arjun: analisado com método registrado como `GET`, descobrindo `q`. [/tmp/reconctx-benchmark-v1/raw/arjun-get/native-output.json:2,11-14 — `/http:~1~1127.0.0.1:18080~1api~1search/method`, `/http:~1~1127.0.0.1:18080~1api~1search/params/0`]

Nenhuma URL/origin da saída GAU coincide com os origins do Katana ou Arjun.

### Sobreposição apenas lexical de nomes de parâmetros

Se “entidade” for apenas o **nome desacoplado do endpoint**, há ainda:

- `q`: Katana observou `q` em `/search?q=seed`; Arjun descobriu `q` em `/api/search`. São endpoints diferentes.  
  [/tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl:5]  
  [/tmp/reconctx-benchmark-v1/raw/arjun-get/native-output.json:12-14]

- `id`: Katana observou `id` em `/api/users?id=1`; Arjun descobriu `id` em `/api/update` e `/api/json`. Também são endpoints diferentes.  
  [/tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl:4]  
  [/tmp/reconctx-benchmark-v1/raw/arjun-post-form/native-output.json:12-15]  
  [/tmp/reconctx-benchmark-v1/raw/arjun-json/native-output.json:14-17]

Essas colisões lexicais **não foram promovidas a uma mesma entidade endpoint+parâmetro**.

---

## 5. Parâmetros descobertos pelo Arjun, location e method

| Endpoint | Parâmetros | Method registrado | Location sustentada pelo RAW | Citação |
|---|---|---|---|---|
| `http://127.0.0.1:18080/api/search` | `q` | `GET` | **Não explicitada.** É compatível com query por ser GET, mas o RAW não contém um campo `location`; portanto, location fica desconhecida. | [/tmp/reconctx-benchmark-v1/raw/arjun-get/native-output.json:2,11-14 — `/http:~1~1127.0.0.1:18080~1api~1search/method`, `/http:~1~1127.0.0.1:18080~1api~1search/params/0`] |
| `http://127.0.0.1:18080/api/update` | `id`, `name` | `POST` | **Desconhecida.** Não há `Content-Type` ou request body no arquivo. Não inferi “form” a partir do nome do diretório. | [/tmp/reconctx-benchmark-v1/raw/arjun-post-form/native-output.json:2,11-15 — `/http:~1~1127.0.0.1:18080~1api~1update/method`, `/http:~1~1127.0.0.1:18080~1api~1update/params/0`, `/params/1`] |
| `http://127.0.0.1:18080/api/json` | `filter`, `id` | `JSON` | **Corpo JSON**, sustentado pelo método/modo `JSON` e `Content-Type: application/json`. O verbo HTTP subjacente não é registrado. | [/tmp/reconctx-benchmark-v1/raw/arjun-json/native-output.json:2,8-16 — `/http:~1~1127.0.0.1:18080~1api~1json/method`, `/http:~1~1127.0.0.1:18080~1api~1json/headers/Content-Type`, `/params/0`, `/params/1`] |
| `http://127.0.0.1:18080/api/no-params` | Nenhum | Não registrado | Não registrada | [/tmp/reconctx-benchmark-v1/raw/arjun-zero/stdout.raw:6-15] |

Os parâmetros positivos foram classificados pelo Arjun com base em **reflexão do valor**.  
[/tmp/reconctx-benchmark-v1/raw/arjun-get/stdout.raw:21-22]  
[/tmp/reconctx-benchmark-v1/raw/arjun-post-form/stdout.raw:25-27]  
[/tmp/reconctx-benchmark-v1/raw/arjun-json/stdout.raw:20-22]

---

## 6. Falhas, parcialidade e coverage gaps

### Falhas explícitas

- **Nenhuma execução pode ser declarada como fatalmente falha** com base no RAW.
- Não há exit codes ou estados finais estruturados, portanto o sucesso integral também não pode ser provado.

### GAU

- Houve warning de erro ao ler `[HOME]/.gau.toml`; a própria mensagem afirma que o GAU continuou com configuração default. Isso é uma **degradação de configuração**, não evidência de falha fatal. [/tmp/reconctx-benchmark-v1/raw/gau/stderr.raw:1]
- O log mostra apenas `page=0` para `otx` e `urlscan`; não há conclusão por provider, paginação adicional ou indicação de outras fontes. [/tmp/reconctx-benchmark-v1/raw/gau/stderr.raw:2-3]
- A saída tem 3 linhas, mas somente 2 URLs únicas, e não registra provider ou timestamp por URL. [/tmp/reconctx-benchmark-v1/raw/gau/native-output.txt:1-3]

### Katana

- Os seis registros têm HTTP 200 e o `stderr` está vazio.
- Coverage gap em relação à união dos resultados: `/api/update`, `/api/json` e `/api/no-params` foram alvos do Arjun, mas não aparecem entre os seis endpoints Katana. Isso indica **diferença de cobertura**, não necessariamente erro do Katana.  
  [/tmp/reconctx-benchmark-v1/raw/arjun-post-form/stdout.raw:6]  
  [/tmp/reconctx-benchmark-v1/raw/arjun-json/stdout.raw:6]  
  [/tmp/reconctx-benchmark-v1/raw/arjun-zero/stdout.raw:6]

### Arjun

- `/api/no-params` terminou com “No parameters were discovered”; isso é um resultado negativo, não uma falha de tool. [/tmp/reconctx-benchmark-v1/raw/arjun-zero/stdout.raw:6-15]
- Não há saída nativa estruturada para essa run entre os arquivos RAW disponíveis.
- Os achados positivos se baseiam somente em reflexão; não há reteste independente, request/response completo ou controle contra falso positivo.
- A location de GET/POST não está explicitamente capturada, e o modo `JSON` não informa o verbo HTTP real.
- Os `stderr.raw` das quatro runs Arjun estão vazios, mas stderr vazio não substitui exit code.

---

## 7. Candidatos excluídos do Arjun e motivos

**Indisponível no RAW.**

Os logs apresentam apenas quantidades “extracted … parameters from response for testing” e os parâmetros finalmente detectados:

| Run | Extraídos para teste | Detectados | Identidades/motivos dos não detectados |
|---|---:|---:|---|
| `/api/no-params` | 7 | 0 | Desconhecidos [/tmp/reconctx-benchmark-v1/raw/arjun-zero/stdout.raw:6-15] |
| `/api/search` | 6 | 1 (`q`) | Desconhecidos [/tmp/reconctx-benchmark-v1/raw/arjun-get/stdout.raw:6-22] |
| `/api/update` | 6 | 2 (`id`, `name`) | Desconhecidos [/tmp/reconctx-benchmark-v1/raw/arjun-post-form/stdout.raw:6-27] |
| `/api/json` | 6 | 2 (`filter`, `id`) | Desconhecidos [/tmp/reconctx-benchmark-v1/raw/arjun-json/stdout.raw:6-22] |

Uma subtração simples produziria diferenças de `7`, `5`, `4` e `4`, mas o RAW não diz que essas diferenças representam formalmente “candidatos excluídos”, nem fornece nomes ou razões. Portanto, **não reporto 20 exclusões como fato**.

Também não existe registro de quais endpoints Katana/GAU foram candidatos à seleção do Arjun e por que alguns teriam sido omitidos.

---

## 8. Evidence IDs

Não há campos ou identificadores denominados `Evidence ID` nos arquivos RAW lidos.

| Resposta | Evidence ID |
|---|---|
| 1 | **Indisponível** |
| 2 | **Indisponível** |
| 3 | **Indisponível** |
| 4 | **Indisponível** |
| 5 | **Indisponível** |
| 6 | **Indisponível** |
| 7 | **Indisponível** |
| 8 | **Indisponível** |
| 9 | **Indisponível** |
| 10 | **Indisponível** |

As referências usadas são somente caminho, linha e, quando aplicável, JSON Pointer.

---

## 9. Lacunas que devem orientar a próxima execução

1. **Capturar uma declaração/snapshot de escopo**, para determinar quais dos três origins são realmente in-scope.
2. **Registrar o mapeamento canônico de assets**, se houver relação entre `127.0.0.1:18080`, `fixture.test` e `finance.fixture.test`.
3. **Validar atualmente as URLs históricas**, somente após confirmar autorização/escopo; nesta condição elas não foram acessadas.
4. **Alimentar o Katana com seeds adicionais** ou documentar por que `/api/update`, `/api/json` e `/api/no-params` não estavam alcançáveis pelo crawl.
5. **Capturar comandos, argumentos, versões, timestamps finais e exit codes** de todas as tools.
6. **GAU:** registrar todos os providers habilitados, status/erro por provider, páginas consultadas, URL→provider e timestamp histórico da observação.
7. **Arjun:** guardar a lista completa de candidatos, decisão testado/rejeitado, motivo, location, verbo HTTP real, request/response de baseline e de confirmação.
8. **Produzir resultado estruturado também para a run sem parâmetros**, em vez de somente stdout.
9. **Introduzir IDs de evidência estáveis** que liguem asset, URL, endpoint, parâmetro, source family e observação.
10. **Executar validação de segurança separada e autorizada** para qualquer hipótese; reflexão de parâmetro, isoladamente, não confirma vulnerabilidade.

---

## 10. Vulnerabilidade confirmada

**Não há vulnerabilidade confirmada nos dados RAW.**

O que existe:

- Endpoints respondendo HTTP 200.
- Parâmetros detectados pelo Arjun com base em reflexão do valor.
- Um warning de configuração do GAU.

Reflexão de parâmetro não demonstra, por si só, XSS, injeção, bypass ou qualquer impacto de segurança.  
[/tmp/reconctx-benchmark-v1/raw/arjun-get/stdout.raw:21-22]  
[/tmp/reconctx-benchmark-v1/raw/arjun-post-form/stdout.raw:25-27]  
[/tmp/reconctx-benchmark-v1/raw/arjun-json/stdout.raw:20-22]

A conclusão é apenas **“nenhuma vulnerabilidade confirmada neste RAW”**, não “o alvo não possui vulnerabilidades”.

---

# Arquivos exatos lidos e bytes

Soma lógica dos tamanhos, contando cada arquivo uma vez: **9.332 bytes**.

```text
2355  /tmp/reconctx-benchmark-v1/raw/katana/stdout.raw
   0  /tmp/reconctx-benchmark-v1/raw/katana/stderr.raw
2355  /tmp/reconctx-benchmark-v1/raw/katana/native-output.jsonl
 324  /tmp/reconctx-benchmark-v1/raw/gau/stderr.raw
  90  /tmp/reconctx-benchmark-v1/raw/gau/native-output.txt
 413  /tmp/reconctx-benchmark-v1/raw/arjun-zero/stdout.raw
   0  /tmp/reconctx-benchmark-v1/raw/arjun-zero/stderr.raw
 804  /tmp/reconctx-benchmark-v1/raw/arjun-post-form/stdout.raw
   0  /tmp/reconctx-benchmark-v1/raw/arjun-post-form/stderr.raw
 541  /tmp/reconctx-benchmark-v1/raw/arjun-post-form/native-output.json
 671  /tmp/reconctx-benchmark-v1/raw/arjun-json/stdout.raw
   0  /tmp/reconctx-benchmark-v1/raw/arjun-json/stderr.raw
 637  /tmp/reconctx-benchmark-v1/raw/arjun-json/native-output.json
 623  /tmp/reconctx-benchmark-v1/raw/arjun-get/stdout.raw
   0  /tmp/reconctx-benchmark-v1/raw/arjun-get/stderr.raw
 519  /tmp/reconctx-benchmark-v1/raw/arjun-get/native-output.json
-----
9332  total
```

Nenhum arquivo foi criado ou modificado.

---

## Unknowns explícitos

- Quais origins são formalmente in-scope.
- Se os origins públicos e o origin local representam o mesmo asset.
- Estado atual das duas URLs provenientes do GAU.
- Provider exato que forneceu cada URL histórica.
- Exit codes e completude real de cada execução.
- Lista e motivos dos candidatos excluídos pelo Arjun.
- Location do parâmetro GET e dos parâmetros POST.
- Verbo HTTP subjacente ao modo Arjun `JSON`.
- Completude do crawl Katana e da coleta GAU.
- Existência de vulnerabilidades além do que foi observado.

## Autoauditoria de alegações sem suporte

- **Não** classifiquei os três origins como confirmadamente in-scope.
- **Não** associei `127.0.0.1` a `fixture.test`.
- **Não** afirmei que as URLs históricas estão offline.
- **Não** atribuí cada URL histórica a OTX ou urlscan.
- **Não** inferi “POST form” pelo nome do diretório.
- **Não** converti diferenças de contagem do Arjun em candidatos excluídos identificados.
- **Não** tratei arquivos duplicados da mesma tool como evidência independente.
- **Não** tratei warning do GAU ou resultado negativo do Arjun como falha fatal.
- **Não** tratei reflexão de parâmetros como vulnerabilidade confirmada.