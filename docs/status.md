# La commande `gaal status` — Guide de lecture

> **Pour qui ?** Ce guide s'adresse à quelqu'un qui n'a jamais utilisé `gaal`
> et veut comprendre ce qu'affiche `gaal status` sans avoir à lire le code source.

---

## Principe général

`gaal` lit un fichier de configuration (`gaal.yaml`) qui déclare trois types de ressources à gérer sur ta machine :

| Type | Ce que c'est |
|------|-------------|
| **Repositories** | Des dépôts de code (git, svn, hg…) à cloner et maintenir à jour localement |
| **Skills** | Des collections de fichiers `SKILL.md` à installer dans les répertoires de tes agents IA (GitHub Copilot, Claude, Cursor…) |
| **MCP Configs** | Des entrées de serveur MCP (*Model Context Protocol*) à injecter dans les fichiers de configuration JSON de tes agents |

La commande `gaal status` **ne fait rien** — elle se contente de **lire l'état actuel du disque** et de te dire si ce que tu as déclaré dans `gaal.yaml` correspond à ce qui est réellement installé.

Pour synchroniser (installer / mettre à jour), tu utilises `gaal sync`.

---

## Structure de l'écran

L'affichage est divisé en **quatre sections** présentées l'une après l'autre.

---

### Section 1 — Repositories

```
── Repositories  (N) ──
┌──────────────┬──────┬────────────────┬──────────────────┐
│ PATH         │ TYPE │ STATUS         │ VERSION / URL    │
```

| Colonne | Signification |
|---------|--------------|
| **PATH** | Chemin local où le dépôt est (ou devrait être) cloné, relatif au répertoire courant |
| **TYPE** | Protocole utilisé : `git`, `hg`, `svn`, `bzr`, `tar`, `zip` |
| **STATUS** | État du dépôt (voir tableau ci-dessous) |
| **VERSION / URL** | Pour les dépôts clonés : version actuelle + version voulue. Pour les dépôts absents : URL source |

**Valeurs possibles de STATUS :**

| Icône | Label | Signification |
|-------|-------|--------------|
| ✓ | `synced` | Cloné et à la bonne version |
| ⚠ | `dirty` | Cloné mais la version locale ne correspond pas à ce que demande la config |
| ~ | `not cloned` | Pas encore téléchargé sur le disque — fait un `gaal sync` |
| ? | `unmanaged` | Présent sur le disque mais non déclaré dans la config |
| ✗ | `error` | Erreur lors de la vérification (permissions, réseau…) |

---

### Section 2 — Skills

```
── Skills  (N) ──
┌──────────────────┬────────────┬───────────┬────────────┬──────────────┐
│ SKILL            │ SOURCE     │ SCOPE     │ STATUS     │ INSTALLED IN │
```

C'est la section la plus importante si tu travailles avec des agents IA.

#### Comprendre les concepts clés

Un **skill** est un dossier contenant un fichier `SKILL.md` que les agents IA lisent pour obtenir des instructions spécialisées (ex. : "comment écrire du React performant"). `gaal` s'occupe de copier ces dossiers dans les bons répertoires de tes agents.

Une **source** est le dépôt GitHub (ou chemin local) depuis lequel `gaal` télécharge les skills. Par exemple `vercel-labs/agent-skills` est un dépôt GitHub public.

#### Colonnes

| Colonne | Signification |
|---------|--------------|
| **SKILL** | Nom du skill (extrait du `SKILL.md`) |
| **SOURCE** | D'où vient ce skill : dépôt GitHub (`owner/repo`) ou chemin local |
| **SCOPE** | `global` = installé dans `~/.copilot/skills` (pour tous tes projets) · `workspace` = installé dans `.github/skills` (uniquement pour ce projet) |
| **STATUS** | État de l'installation (voir tableau ci-dessous) |
| **INSTALLED IN** | Dans quels agents le skill est actuellement présent sur le disque |

**Valeurs possibles de STATUS :**

| Icône | Label | Signification |
|-------|-------|--------------|
| ✓ | `synced` | Le skill est installé et ses fichiers correspondent exactement à la source |
| ⚠ | `dirty` | Le skill est installé mais des fichiers ont été modifiés localement depuis la dernière sync |
| ~ | `partial` | Déclaré dans la config mais pas encore installé — fait un `gaal sync` |
| ? | `unmanaged` | Trouvé sur le disque dans un répertoire agent mais non déclaré dans la config |
| ✗ | `error` | Erreur (source non cachée, agent inconnu…) |

**Valeurs possibles de INSTALLED IN :**

| Valeur | Signification |
|--------|--------------|
| `all` (vert) | Installé dans tous les agents ciblés par la config |
| `none` (jaune) | Non encore installé dans aucun agent — `gaal sync` requis |
| liste de noms | Installé uniquement dans ces agents (sous-ensemble des agents ciblés) |

> **Astuce lecture :** une ligne `~ partial | none` signifie que le skill est déclaré dans
> `gaal.yaml` mais que sa source n'a pas encore été téléchargée localement. Lance
> `gaal sync` pour corriger ça.

#### Exemple de lecture

```
│ react-best-practices │ vercel-labs/agent-skills │ workspace │ ✓ synced  │ all  │
```
→ Le skill `react-best-practices`, tiré du dépôt GitHub `vercel-labs/agent-skills`, est installé dans le répertoire `.github/skills` du projet et est synchronisé dans tous les agents ciblés.

```
│ canvas-design        │ anthropics/skills        │ global    │ ✓ synced  │ all  │
```
→ Ce skill est installé **globalement** (`~/.copilot/skills`) : il sera disponible dans tous tes projets, pas seulement celui-ci.

```
│ vercel-composition…  │ vercel-labs/agent-…      │ workspace │ ~ partial │ none │
```
→ Ce skill est dans ta config mais pas encore téléchargé. Lance `gaal sync`.

---

### Section 3 — MCP Configs

```
── MCP Configs  (N) ──
┌────────────┬────────────┬───────────────────────────────────┐
│ NAME       │ STATUS     │ TARGET                            │
```

| Colonne | Signification |
|---------|--------------|
| **NAME** | Nom de l'entrée MCP telle que déclarée dans `gaal.yaml` |
| **STATUS** | État de la configuration (voir tableau ci-dessous) |
| **TARGET** | Fichier JSON de configuration de l'agent où l'entrée doit être injectée |

**Valeurs possibles de STATUS :**

| Icône | Label | Signification |
|-------|-------|--------------|
| ✓ | `present` | L'entrée est présente dans le fichier cible et correspond à la config |
| ⚠ | `dirty` | L'entrée existe mais a été modifiée localement depuis la dernière sync |
| ~ | `absent` | L'entrée n'est pas dans le fichier cible — `gaal sync` requis |
| ✗ | `error` | Impossible de lire/écrire le fichier cible |

---

### Section 4 — Supported Agents

```
── Supported Agents  (N) ──
┌───────────────┬───────────┬────────────────────┬───────────────────┬───────────────────┐
│ AGENT         │ INSTALLED │ PROJECT SKILLS DIR │ GLOBAL SKILLS DIR │ PROJECT MCP CONFIG│
```

Cette section est **informative uniquement** — elle te montre quels agents IA `gaal` connaît et lesquels sont détectés comme présents sur ta machine.

| Colonne | Signification |
|---------|--------------|
| **AGENT** | Identifiant de l'agent (ex. `github-copilot`, `cursor`, `claude-code`) |
| **INSTALLED** | `✓` si le répertoire de configuration de l'agent existe sur la machine · `—` si absent |
| **PROJECT SKILLS DIR** | Répertoire (relatif au projet) où `gaal` installe les skills workspace pour cet agent |
| **GLOBAL SKILLS DIR** | Répertoire absolu (`~/ …`) où `gaal` installe les skills globaux pour cet agent |
| **PROJECT MCP CONFIG** | Fichier JSON de l'agent où les entrées MCP sont injectées |

> `gaal` ne synchronise vers un agent que s'il est **installé** (`✓`). Les agents
> marqués `—` sont ignorés lors du `gaal sync`.

---

## Statuts en un coup d'œil

| Icône | Couleur | Signification rapide |
|-------|---------|---------------------|
| ✓ synced / present | vert | Tout est à jour |
| ⚠ dirty | jaune | Modifié localement depuis la dernière sync |
| ~ partial / not cloned / absent | jaune | Déclaré mais pas encore installé → `gaal sync` |
| ? unmanaged | cyan | Présent sur le disque mais pas dans la config |
| ✗ error | rouge | Problème à corriger (voir le message d'erreur) |

---

## Workflow typique

```
1. Tu édites gaal.yaml          → ajoute/modifie des ressources
2. gaal status                  → vois ce qui est désynchronisé
3. gaal sync                    → installe / met à jour tout
4. gaal status                  → confirme que tout est ✓ synced
```
