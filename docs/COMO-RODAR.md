# Como rodar

Este guia cobre os quatro jeitos de executar o projeto, do mais rápido ao mais completo.

| Quero… | Use | Precisa de |
|---|---|---|
| Ver a interface, sem Matrix | [Modo dev](#1-modo-dev-sem-matrix) | Go |
| Testar com Element e Synapse reais, só no PC | [Stack local](#2-stack-local-só-no-pc) | Docker |
| Testar também no celular ou com outras pessoas | [Stack com Tailscale](#3-stack-com-tailscale-celular-e-outras-pessoas) | Docker, Tailscale |
| Publicar para uso real | [Produção](#4-produção) | Homeserver e bot |

## Requisitos
- **Go 1.27+** para o modo dev e os testes.
- **Docker** com o plugin `docker compose`, mais `curl` e `jq`, para os scripts de stack.
- **Tailscale** (opcional), só para acessar pelo celular.
- [Task](https://taskfile.dev) (opcional): os atalhos `task ...` abaixo chamam os mesmos comandos.

## 1. Modo dev (sem Matrix)
O app usa um cliente Matrix falso. Não precisa de homeserver, e todo usuário é tratado como professor.

```bash
DEV_MODE=true go run ./cmd/server     # ou: task dev
```
Abra `http://localhost:8080/?roomId=!sala:localhost`.

## 2. Stack local (só no PC)
Sobe Synapse, Element Web e o app em modo real, e prepara uma turma de teste.

```bash
./scripts/dev-local.sh      # ou: task stack:local
```

O script cria:
- 1 professor, 3 alunos e o bot de frequência;
- a sala **SD** com o widget já adicionado;
- um **PIN** de 6 dígitos para a sala.

Ao terminar, ele imprime os endereços, os usuários, as senhas e o PIN. Repita o comando quando quiser: o que já existe é reaproveitado.

Abra o Element Web em **http://localhost:8081**. Entre como o professor e abra a sala SD. O widget fica em **Informações da sala → Extensões**.

| Serviço | Endereço |
|---|---|
| Element Web | http://localhost:8081 |
| Synapse (homeserver) | http://localhost:8008 |
| App / widget | http://localhost:8080 |

Contas criadas (as senhas seguem o padrão `senha-<usuário>-123`):

| Conta | Usuário |
|---|---|
| Professor | `professor-sd` |
| Alunos | `aluno1-sd`, `aluno2-sd`, `aluno3-sd` |
| Bot | `frequenciabot` |

## 3. Stack com Tailscale (celular e outras pessoas)
O Element Web exige HTTPS fora do `localhost`. O Tailscale fornece um endereço HTTPS válido.

```bash
./scripts/dev-stack.sh      # ou: task stack
```
O script publica três portas pelo `tailscale serve`:

| Porta | Serviço |
|---|---|
| 443 | App / widget |
| 8443 | Synapse (homeserver) |
| 10000 | Element Web |

Os endereços finais (`https://<host>.ts.net...`) aparecem no resumo do script.

- **Só você (dispositivos na sua conta Tailscale):** instale o Tailscale no celular, entre na mesma conta e use os endereços acima.
- **Outras pessoas sem instalar nada:** use o Funnel, que torna os endereços públicos.
  ```bash
  tailscale funnel --bg --https=443   http://localhost:8080
  tailscale funnel --bg --https=8443  http://localhost:8008
  tailscale funnel --bg --https=10000 http://localhost:8081
  tailscale funnel status
  tailscale funnel reset    # desliga tudo
  ```
  **Atenção:** o Synapse deste stack tem cadastro aberto (`enable_registration: true`). Com o Funnel ligado, qualquer pessoa com o link pode criar conta. Use só em testes curtos e rode `tailscale funnel reset` depois.

Se o `serve` falhar, habilite o Serve e o HTTPS no painel do Tailscale. O CLI mostra o link.

### No app Element do celular
1. Em **Editar** (homeserver), informe `https://<host>.ts.net:8443` e entre com um aluno ou o professor.
2. Abra a sala SD e o widget.
3. O Element Android não confirma a identidade pelo protocolo OpenID. O widget então pede o **PIN da sala**. O professor vê o PIN ao usar o botão **Gerar novo PIN** na aba de configuração.

## Comandos comuns dos scripts
Valem para `dev-local.sh` e `dev-stack.sh`.

| Comando | Efeito |
|---|---|
| `./scripts/dev-local.sh` | Sobe e prepara tudo (idempotente) |
| `./scripts/dev-local.sh --new-pin` | Gera um PIN novo |
| `./scripts/dev-local.sh clean` | **Apaga tudo** e recria do zero, pedindo para digitar `apagar` |
| `./scripts/dev-local.sh clean --yes` | O mesmo, sem confirmação |
| `./scripts/dev-stack.sh --no-tailscale` | Equivale ao `dev-local.sh` |
| `./scripts/<script> --help` | Mostra a ajuda |

O `clean` remove os containers, os volumes do Synapse e do app (contas, salas e chamadas), a imagem do app e a pasta `.dev-stack/`. Só afeta os recursos Docker deste projeto.

Depois de um `clean`, limpe também o que os clientes guardam:
- **Navegador:** apague os dados do site do Element, ou use uma janela anônima.
- **App no celular:** saia da conta e remova-a.

Estado local do script (PIN atual): `.dev-stack/state.env`, ignorado pelo git.

## 4. Produção
Veja a seção "Implantação em Produção" do [README](../README.md). Resumo:

- Crie uma conta de bot no homeserver e gere o token de acesso.
- Configure as variáveis: `MATRIX_HOMESERVER_URL`, `MATRIX_USER_ID`, `MATRIX_ACCESS_TOKEN` e `DEV_MODE=false`.
- Sirva o app por HTTPS e adicione o widget à sala. O professor convida o bot como qualquer usuário. Se `MATRIX_AUTOJOIN_SERVERS` estiver definida, o bot só aceita convites desses servidores.
- Defina `PIN_SESSION_TTL` (padrão `24h`) para mudar a duração do acesso por PIN.

## Testes
```bash
go test -race ./...          # ou: task test
golangci-lint run            # ou: task lint
```

## Problemas comuns
| Sintoma | Causa e solução |
|---|---|
| "Element does not support this browser" | O Element Web foi aberto por `http` num IP da rede. Use `localhost` ou o endereço HTTPS do Tailscale. |
| Porta 8080 ocupada | Outro processo (um `go run`?) usa a porta. Pare-o e rode de novo. |
| `Serve is not enabled` | Habilite o Serve no painel do Tailscale pelo link que o CLI mostra. |
| Widget mostra 403 mesmo para o professor | O bot não está na sala. Convide `@frequenciabot:localhost`. Ele entra sozinho. |
| Tela "Confirme sua identidade" ao entrar | A conta já tinha chaves de verificação em outro dispositivo. Use outra conta ou confirme pelo outro dispositivo. |
| Página não carrega depois de reiniciar o PC | O IP da rede mudou. Use o endereço Tailscale, que não muda. |
| `M_LIMIT_EXCEEDED` ao convidar | O Synapse limita convites por usuário. Espere um pouco ou use outra conta. |
| Contas antigas com senha diferente | Rode `./scripts/dev-local.sh clean` para recomeçar do zero. |
