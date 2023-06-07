// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.13;

import "forge-std/Script.sol";
import {NativeToken} from "src/NativeToken.sol";

contract EdgeSetup is Script {
    /// @notice kept as a reminder for the user.
    bytes32 private constant EDGE_VERSION = keccak256("0.9.0");

    address private constant NATIVE_TOKEN = 0xc2d8B4ce4FF3D9ead10C790b3096fE0dB29E8E69;

    address payable private to;
    address private from;
    address[] private validators;

    function setUp() public {
        to = payable(vm.envAddress("TO_ADDRESS"));
        from = vm.envAddress("FROM_ADDRESS");
        validators.push(address(0x1090558c901e2921eCFF6Fa46366BB3f3BeA8eF0));
        validators.push(address(0x0920b133EFA338C5cD0ceA802Da13AE7792Ec090));
        validators.push(address(0x570E53A34d94b156C13689b3fDC3Ef75714CDD9d));
        validators.push(address(0xE28E2b0A90Ee0B90b79d6B6629ebd7EDf5016dAB));
    }

    function run() public {
        vm.startBroadcast();
        vm.stopBroadcast();
    }

    function mintNativeTokens() public {
        NativeToken token = NativeToken(0x6FE03c2768C9d800AF3Dedf1878b5687FE120a27);
        vm.startBroadcast();
        token.mint(address(this), 10e18);
        for (uint256 i = 0; i < validators.length; i++) {
            token.mint(validators[i], 10e18);
        }

        vm.stopBroadcast();
    }

    function deployNativeToken() public {
        vm.startBroadcast();
        NativeToken token = new NativeToken();
        for (uint256 i = 0; i < validators.length; i++) {
            token.mint(validators[i], 100 * 10 ** token.decimals());
        }
        vm.stopBroadcast();
    }
}
