# Configuration file for the Sphinx documentation builder.
# https://www.sphinx-doc.org/en/master/usage/configuration.html

import json
from pathlib import Path

# -- Project information -----------------------------------------------------

project = 'Foundry VTT Package CORS Proxy'
copyright = '2026, Ulenar of Mondarth; 2026, Melih Birim'
author = 'Ulenar of Mondarth'
# The release's version, which semantic-release sets in package.json. The container sees it at
# /project/package.json (compose.yaml).
release = json.loads(Path('/project/package.json').read_text())['version']

# -- General configuration ---------------------------------------------------

extensions = ['myst_parser', 'sphinx.ext.todo', 'sphinxcontrib.mermaid']

templates_path = ['_templates']
exclude_patterns = ['_build', 'Thumbs.db', '.DS_Store']

# -- MyST (Markdown) ---------------------------------------------------------
# https://myst-parser.readthedocs.io/en/latest/configuration.html

myst_enable_extensions = ['colon_fence', 'deflist', 'substitution']
myst_heading_anchors = 3

# -- Options for HTML output -------------------------------------------------

html_theme = 'furo'
html_static_path = ['_static']

# -- TODOs are shown in the output and listed on the front page. They do not count as
# warnings, so they do not fail the build.

todo_include_todos = True
todo_emit_warnings = False
