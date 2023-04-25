# Edge sandbox

This repository is aimed to test blockchain interactions for a deployed [Polygon Supernet](https://wiki.polygon.technology/docs/supernets/get-started/what-are-supernets)



## client_test.go

**Setup**

- A running Supernets Instance
    - Can be [local](https://wiki.polygon.technology/docs/supernets/operate/supernets-local-deploy-supernet)
- `.env` -- Environment variables neede for testing

* use [polycli](https://github.com/maticnetwork/polygon-cli) to generate wallets and save the wallet keys/addresses

```polycli wallet create --words 12 --language english | jq '.Addresses[:2]' > rootchain-wallet.json```

* Use one of the addresses as part of genesis. 

``` ./polygon-edge geneisis {options} --premine <ADDRESS>:1000000000000000000000000000 ```

**Running**
1) `make test-verbose`



