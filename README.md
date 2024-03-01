# README (staplegun)

A stupid-simple templating engine for Go.

Documentation TBD.

## Syntax

```html

{{ staplegun parent }}

{{ staplegun child }}

{{ staplegun define_block <blockname> }}
{{ staplegun end }}

{{ staplegun import_file <filename> }}

{{ staplegun insert_block <blockname> }}


```