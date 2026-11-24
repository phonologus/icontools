(procedure_declaration
  "procedure" @context
  name: (identifier) @name
  parameters: (parameter_list) @context.extra) @item

(record_declaration
  "record" @context
  name: (identifier) @name) @item

(global_declaration
  "global" @context
  (identifier) @name) @item

(link_declaration
  "link" @context
  [(identifier) (string)] @name) @item
