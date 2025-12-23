##@ Python/PyPI build targets

CLEAN_TARGETS += ./python/dist ./python/*.egg-info

.PHONY: python-publish
python-publish: ## Publish the python packages
	cd ./python && \
	sed -i "s/version = \".*\"/version = \"$(GIT_TAG_VERSION)\"/" pyproject.toml && \
	uv build && \
	uv publish