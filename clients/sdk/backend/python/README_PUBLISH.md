Publishing

Build a wheel and upload to PyPI using `twine`:

  python3 -m pip install --upgrade build twine
  python3 -m build
  python3 -m twine upload dist/*

You can use the helper script `.scripts/publish-python.sh` which will run build and upload (interactive credentials).

