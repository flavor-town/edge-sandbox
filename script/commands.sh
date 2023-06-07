#!/bin/bash
source .env


sendETH() {
    cast send --rpc-url $EDGE_URL--value 1ether $TO_ADDRESS --legacy --private-key $PRIVATE_KEY
}


deployNFT(){
    nft_bytecode=`forge inspect MyNFT bytecode`
    cast send --legacy --rpc-url $EDGE_URL --private-key $PRIVATE_KEY --create $nft_bytecode
}


mintNFT(){
    amount=$1
    nft_address="0xdB88B0D1BD63EB8d0418ca530FEB114A7331a6c5"
    functionSig="mint(address,uint256)"
    functionParams="$TO_ADDRESS $amount"

    cast send --legacy --rpc-url $EDGE_URL --private-key $PRIVATE_KEY $nft_address $functionSig $functionParams
}

nftBalance() {
    to=$1
    cast call --legacy --rpc-url $EDGE_URL --private-key $PRIVATE_KEY 0xdB88B0D1BD63EB8d0418ca530FEB114A7331a6c5 "balanceOf(address)(uint256)" $to
}


# sendETH
# deployNFT
# mintNFT 1
# nftBalance $TO_ADDRESS

