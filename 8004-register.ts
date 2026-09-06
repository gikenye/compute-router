import {
  createPublicClient,
  createWalletClient,
  encodeFunctionData,
  formatUnits,
  http,
  type Abi,
} from "viem";
import { celo } from "viem/chains";
import { privateKeyToAccount } from "viem/accounts";

const agentURI =
  process.env.AGENT_URI ??
  "https://www.computerouter.wtf/.well-known/agent-card.json";
const rpcURL = process.env.CELO_RPC_URL ?? "https://forno.celo.org";
const privateKey = process.env.AGENT_OPERATOR_PRIVATE_KEY;

if (!privateKey) {
  throw new Error("AGENT_OPERATOR_PRIVATE_KEY must be set in the environment");
}

const account = privateKeyToAccount(
  privateKey.startsWith("0x")
    ? (privateKey as `0x${string}`)
    : (`0x${privateKey}` as `0x${string}`),
);
const publicClient = createPublicClient({
  chain: celo,
  transport: http(rpcURL),
});
const walletClient = createWalletClient({
  account,
  chain: celo,
  transport: http(rpcURL),
});

const usdtAddress = "0x48065fbbe25f71c9282ddf5e1cd6d6a887483d5e";
const usdtFeeCurrency = "0x0e2a3e05bc9a16f5292a6170456a710cb89c6f72";
const registryAddress = "0x8004A169FB4a3325136EB29fA0ceB6D2e539a432";

const erc20ABI = [
  {
    name: "balanceOf",
    type: "function",
    stateMutability: "view",
    inputs: [{ name: "account", type: "address" }],
    outputs: [{ name: "", type: "uint256" }],
  },
] as const satisfies Abi;

const registryABI = [
  {
    name: "register",
    type: "function",
    stateMutability: "nonpayable",
    inputs: [{ name: "agentURI", type: "string" }],
    outputs: [{ name: "", type: "uint256" }],
  },
] as const satisfies Abi;

async function main(): Promise<void> {
  const documentResponse = await fetch(agentURI);
  if (!documentResponse.ok) {
    throw new Error(
      `agent document check failed: ${agentURI} returned HTTP ${documentResponse.status}`,
    );
  }

  const balance = await publicClient.readContract({
    address: usdtAddress,
    abi: erc20ABI,
    functionName: "balanceOf",
    args: [account.address],
    authorizationList: undefined,
  });
  console.log("wallet", account.address);
  console.log("usdtBalance", formatUnits(balance, 6));

  const data = encodeFunctionData({
    abi: registryABI,
    functionName: "register",
    args: [agentURI],
  });
  const gas = await publicClient.estimateGas({
    account,
    to: registryAddress,
    data,
    feeCurrency: usdtFeeCurrency,
    type: "cip64",
  });
  console.log("gasEstimate", gas.toString());

  const hash = await walletClient.writeContract({
    address: registryAddress,
    abi: registryABI,
    functionName: "register",
    args: [agentURI],
    account,
    feeCurrency: usdtFeeCurrency,
    type: "cip64",
    chain: celo,
    gas,
  });
  console.log("txHash", hash);

  const receipt = await publicClient.waitForTransactionReceipt({ hash });
  if (receipt.status !== "success") {
    throw new Error(`registration transaction reverted: ${hash}`);
  }
  console.log("status", receipt.status, "block", receipt.blockNumber.toString());
}

main().catch((error: unknown) => {
  console.error(error);
  process.exitCode = 1;
});
