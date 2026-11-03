# Frequência Matrix Integration & Widget

Integração de chamada e controle de frequência para salas Matrix e Element Web no curso de *Sistemas Distribuídos* (`matrix.dc.ufc.br`), desenvolvida em **Go**.

## Visão Geral

- **Controle de Acesso por Papel na Sala (Teacher-Only)**: Inspeciona o estado da sala Matrix (`m.room.power_levels`). Usuários com nível de poder $\ge 50$ (Moderadores / Professores) têm acesso total. Alunos (nível $0$) recebem tela explicativa de acesso restrito.
- **Configuração da Disciplina**: Definição de dias da semana (ex: Terça/Quinta), data de início e total de períodos letivos planejados no semestre.
- **Quadro de Alunos**: Cadastro e remoção de alunos em duas categorias independentes: **Graduação** e **Pós-Graduação**. Ordenação alfabética insensível a acentos (ex: *Érika* tratada como *Erika*).
- **Chamada Diária Interativa**: Widget visual embutido via HTMX/Tailwind no Element para realização da chamada em tempo real durante a aula.
- **Notificação na Sala**: Ao salvar a chamada, um aviso (`m.notice`) é postado na sala Matrix com o resumo de presenças da data.
- **Exportação XLSX de Fim de Semestre**: Gera planilha Excel `.xlsx` com duas abas (`Graduacao` e `Pos-Graduacao`), alunos em ordem alfabética A-Z, colunas dinâmicas por dia de aula (`P`/`F`), totais e cálculo de aprovação por frequência ($\ge 75\%$).
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

Inicia a aplicação Go, um servidor Matrix local (Conduit) e o cliente Element Web:

```bash
docker compose up -d
```

- **Element Web**: `http://localhost:8081`
- **Matrix Homeserver**: `http://localhost:6167`
- **Widget de Frequência**: `http://localhost:8080`

No Element Web:
1. Registre uma conta no homeserver `http://localhost:6167`.
2. Crie uma sala.
3. No chat da sala, digite:
   ```
   /addwidget http://localhost:8080/?roomId=$matrix_room_id&userId=$matrix_user_id
   ```

---

## Implantação em Produção (`matrix.dc.ufc.br`)

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
│   │   ├── schedule.go         # Algoritmos de agendamento, ordenação com folding e cálculos de presença
│   │   └── schedule_test.go    # Testes unitários do domínio
│   ├── store/
│   │   ├── sqlite.go           # Repositório SQLite puro Go (WAL mode)
│   │   ├── migrations.go       # Executor de migrações embutidas (//go:embed)
│   │   ├── migrations/         # Arquivos SQL versionados (001_initial.sql)
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
│       ├── handler.go          # Handlers HTTP, middleware de autorização e rotas HTMX
│       ├── handler_test.go     # Testes de endpoints HTTP e renderização
│       └── templates/          # Templates HTML embutidos com HTMX e Tailwind
│           ├── layout.html
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
