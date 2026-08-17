# Modos de Busca - Smart Search Service

O sistema agora suporta dois modos de busca que podem ser alterados através da variável de ambiente `SEARCH_MODE`.

## Configuração

Adicione no seu arquivo `.env`:

```env
SEARCH_MODE=sql     # ou fuzzy
```

## Modos Disponíveis

### 1. Modo SQL (Padrão)
```env
SEARCH_MODE=sql
```

**Características:**
- ✅ Mais rápido
- ✅ Busca exata no banco
- ❌ Não tolera erros de digitação
- ❌ Sem ordenação por relevância
- ❌ Busca rígida (precisa ter todos os termos)

**Use quando:**
- Performance é prioridade
- Dados estão bem padronizados
- Usuários digitam corretamente

### 2. Modo Fuzzy
```env
SEARCH_MODE=fuzzy
```

**Características:**
- ✅ Tolerante a erros de digitação
- ✅ Ordenação por score de relevância
- ✅ Busca inteligente
- ✅ Melhor experiência do usuário
- ❌ Ligeiramente mais lento

**Use quando:**
- Qualidade dos resultados é prioridade
- Usuários podem cometer erros de digitação
- Dados podem ter variações

## Exemplos

### Busca: "honda civci 2020"

**Modo SQL:**
- Resultado: ❌ Nenhum (erro em "civci")

**Modo Fuzzy:**
- Resultado: ✅ "Honda Civic 2020" (score: 85%)
- Resultado: ✅ "Honda Civic 2019" (score: 80%)
- Resultado: ✅ "Honda City 2020" (score: 70%)

## Alterando o Modo

1. **Desenvolvimento:** Edite o arquivo `.env`
2. **Produção:** Configure a variável de ambiente `SEARCH_MODE`
3. **Reinicie o serviço** para aplicar as mudanças

```bash
# Modo SQL (padrão)
SEARCH_MODE=sql

# Modo Fuzzy
SEARCH_MODE=fuzzy
```

## Logs

O sistema mostra no log qual modo está sendo usado:
```
Search mode: sql
# ou
Search mode: fuzzy
```

## Performance

- **SQL:** ~5-10ms por busca
- **Fuzzy:** ~15-50ms por busca (dependendo do dataset)

O overhead do fuzzy é mínimo e vale a pena pela melhoria na qualidade dos resultados.