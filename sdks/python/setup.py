from setuptools import setup, find_packages

setup(
    name="anakinscraper",
    version="0.1.0",
    description="Python SDK for AnakinScraper",
    author="AnakinAI",
    url="https://github.com/AnakinAI/anakinscraper-py",
    packages=find_packages(),
    install_requires=["requests>=2.28.0"],
    python_requires=">=3.8",
    license="MIT",
    classifiers=[
        "License :: OSI Approved :: MIT License",
        "Programming Language :: Python :: 3",
    ],
)
