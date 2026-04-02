##@ Python/PyPI build targets

CLEAN_TARGETS += ./python/dist ./python/*.egg-info

.PHONY: python-publish
python-publish: ## Publish the python packages
	cd ./python && \
	sed "s/version = \".*\"/version = \"$(GIT_TAG_VERSION)\"/" pyproject.toml > pyproject.toml.tmp && mv pyproject.toml.tmp pyproject.toml && \
	uv build && \
	uv publish