# Frequência Matrix Integration & Widget

Integração de chamada e controle de frequência para salas Matrix e Element Web no curso de *Sistemas Distribuídos* (`matrix.dc.ufc.br`), desenvolvida em **Go**.

## Visão Geral

- **Controle de Acesso por Papel na Sala (Teacher-Only)**: Inspeciona o estado da sala Matrix (`m.room.power_levels`). Usuários com nível de poder &ge; 50 (Moderadores / Professores) têm acesso total. Alunos (nível 0) recebem tela explicativa de acesso restrito.
- **Configuração da Disciplina**: Definição de dias da semana (ex: Terça/Quinta), data de início e total de períodos letivos planejados no semestre.
- **Quadro de Alunos**: Cadastro e remoção de alunos em duas categorias independentes: **Graduação** e **Pós-Graduação**. Ordenação alfabética insensível a acentos (ex: *Érika* tratada como *Erika*). Alunos removidos ficam marcados como *inativos* no resumo e na planilha, preservando o histórico.
- **Importação por CSV**: Importa a turma de um arquivo `aluno,matricula` (separador `,`, `;` ou tab; com ou sem cabeçalho; UTF-8, Windows-1252 ou UTF-16 do Excel). Reimportar o mesmo arquivo atualiza os alunos em vez de duplicá-los. Há um modelo para download na própria aba.
- **Chamada por faltas**: Todos os alunos começam presentes e o professor toca **apenas em quem faltou**. O widget mostra o contador de faltas do dia e as faltas acumuladas de cada aluno contra o limite. O salvamento é explícito (botão "Salvar chamada").
- **Calendário de aulas**: A data é escolhida entre as aulas previstas (dias da semana configurados), com opção de aula extra. Um dia pode ser marcado como **"sem aula"**: ninguém recebe falta.
- **Notificação na Sala**: Na primeira gravação de uma data, um aviso (`m.notice`) é postado na sala Matrix com o número de faltas. Editar a chamada depois não repete o aviso.
- **Regra de aprovação**: São exigidos **75% de presença** sobre a carga horária total. Com 32 períodos o limite é 8 faltas. O status é *Em dia*, *Atenção* (a partir de 75% do limite) ou *Reprovado por falta*.
- **Exportação XLSX de Fim de Semestre**: Gera planilha Excel `.xlsx` com duas abas (`Graduacao` e `Pos-Graduacao`), datas das aulas por aluno (`P`/`F`, `S/A` para dia sem aula), faltas, % de faltas, faltas restantes, % de frequência e situação.
- **Persistência Confiável e Leve**: SQLite puro Go (`modernc.org/sqlite`, sem dependência de CGO) com modo WAL (`Write-Ahead Logging`) e migrações SQL embutidas no binário.
- **Container Estático Ultraleve**: Imagem Docker multi-stage construída sobre `FROM scratch` executando com usuário não-privilegiado `10001:10001`.

---

## Como Executar Localmente

### 1. Modo de Desenvolvimento Rápido (Dev Mode)

Para testar a interface do widget e a exportação XLSX sem precisar de um servidor Matrix ativo:

```bash
DEV_MODE=true go run ./cmd/server
```

Acesse no seu navegador:
```
http://localhost:8080/?roomId=!demo:matrix.dc.ufc.br&userId=@professor:matrix.dc.ufc.br
```

Para testar o bloqueio de permissão de alunos:
```
http://localhost:8080/?roomId=!demo:matrix.dc.ufc.br&userId=@aluno:matrix.dc.ufc.br
```

### 2. Stack Completo Local via Docker Compose

Inicia a aplicação Go, um servidor Matrix local (Synapse) e o cliente Element Web:

```bash
docker compose up -d
```

- **Element Web**: `http://localhost:8081`
- **Matrix Homeserver (Synapse)**: `http://localhost:8008`
- **Widget de Frequência**: `http://localhost:8080`

> O serviço do app no `docker-compose.yml` usa `DEV_MODE=true`. Depois de alterar o código, reconstrua a imagem com `docker compose up -d --build frequencia`.

No Element Web:
1. Conecte ao homeserver `http://localhost:8008` e crie sua conta/login.
2. Crie uma sala.
3. No chat da sala, digite:
   ```
   /addwidget http://localhost:8080/?roomId=$matrix_room_id&widgetId=$matrix_widget_id&userId=$matrix_user_id
   ```
   No modo dev, o `userId` identifica o professor (um usuário cujo nome contém `prof` ou `teacher`). O widget está no painel da sala, em *Informações da sala → Extensões*.

---

## Implantação em Produção (`matrix.dc.ufc.br`)

Adicione o widget à sala com o comando abaixo (**sem** `userId`):

```
/addwidget https://frequencia.exemplo.ufc.br/?roomId=$matrix_room_id&widgetId=$matrix_widget_id
```

A identidade vem do Element, não da URL: o widget pede ao Element um token OpenID (Widget API, ação `get_openid`) e o envia no header `X-OpenID-Token`. Na primeira vez, o Element pergunta ao professor se permite que o widget verifique a identidade. O token nunca aparece em URLs. O backend valida o token no homeserver e só libera quem tem nível de poder &ge; 50 na sala, ou quem criou a sala (em salas versão 12 o criador não aparece na lista de níveis).

O bot (`MATRIX_USER_ID`) precisa estar na sala para postar os avisos.

Configure as variáveis de ambiente:

| Variável | Descrição | Exemplo |
|---|---|---|
| `PORT` | Porta HTTP do serviço | `8080` |
| `DB_PATH` | Caminho do arquivo SQLite | `/app/data/frequencia.db` |
| `MATRIX_HOMESERVER_URL` | URL do homeserver Matrix | `https://matrix.dc.ufc.br` |
| `MATRIX_USER_ID` | ID do bot da integração | `@frequencia-bot:matrix.dc.ufc.br` |
| `MATRIX_ACCESS_TOKEN` | Token de acesso da conta bot | `syt_...` |
| `DEV_MODE` | `false` em produção | `false` |

---

## Testes Automatizados & Cobertura

O projeto possui mais de 80% de cobertura de código em testes unitários e de integração, executáveis em um único comando sem dependências externas:

```bash
go test -v -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
```

Ou usando Task:
```bash
task test
```

---

## Estrutura do Repositório

```
frequencia-matrix-integration/
├── cmd/
│   └── server/
│       ├── main.go             # Ponto de entrada, injeção de dependências e graceful shutdown
│       ├── config.go           # Carregamento e validação de variáveis de ambiente
│       └── config_test.go      # Testes de configuração e validação
├── internal/
│   ├── domain/
│   │   ├── models.go           # Modelos de domínio imutáveis e validação
│   │   ├── schedule.go         # Datas das aulas e ordenação com remoção de acentos
│   │   ├── calendar.go         # Janela de datas da chamada, data padrão e rótulos
│   │   ├── summary.go          # Faltas, limite de 75% de presença e status do aluno
│   │   └── *_test.go           # Testes unitários do domínio
│   ├── importer/
│   │   └── students_csv.go     # Leitura do CSV [aluno, matrícula] (BOM, separadores, codificações)
│   ├── store/
│   │   ├── sqlite.go           # Repositório SQLite puro Go (WAL mode)
│   │   ├── import.go           # Importação idempotente da turma (upsert por matrícula/nome)
│   │   ├── migrations.go       # Executor de migrações embutidas (//go:embed)
│   │   ├── migrations/         # Arquivos SQL versionados (001_initial.sql, 002_session_cancelled.sql)
│   │   └── sqlite_test.go      # Testes de integração com SQLite in-memory
│   ├── matrix/
│   │   ├── gateway.go          # Interface abstrata do gateway Matrix
│   │   ├── fake_client.go      # Cliente simulado para testes e modo dev
│   │   ├── mautrix_client.go   # Implementação oficial usando mautrix-go
│   │   └── gateway_test.go     # Testes de verificação de papel e tokens
│   ├── export/
│   │   ├── xlsx.go             # Gerador de planilha com 2 abas (Graduação e Pós-Graduação)
│   │   └── xlsx_test.go        # Testes de formatação e ordenação A-Z no XLSX
│   └── web/
│       ├── handler.go          # Rotas, página inicial (shell) e arquivos estáticos
│       ├── auth.go             # Credenciais (header/URL) e autorização de professor
│       ├── handler_*.go        # Setup, alunos, importação, chamada e resumo
│       ├── *_test.go           # Testes de endpoints HTTP e renderização
│       ├── static/widget.js    # Handshake get_openid com o Element, token em header, download
│       └── templates/          # Templates HTML embutidos com HTMX e Tailwind
│           ├── layout.html     # Documento (shell ou página completa)
│           ├── app.html        # Corpo do widget: cabeçalho, abas e conteúdo
│           ├── setup.html
│           ├── roster.html
│           ├── attendance.html
│           ├── summary.html
│           └── unauthorized.html
├── Dockerfile                  # Multi-stage FROM scratch com usuário 10001:10001
├── docker-compose.yml          # Stack local de testes (App + Conduit + Element)
├── Taskfile.yml                # Comandos de teste, build e lint
└── README.md
```
