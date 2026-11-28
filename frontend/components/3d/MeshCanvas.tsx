'use client';

import { Grid, Line, OrbitControls } from '@react-three/drei';
import { Canvas, useFrame } from '@react-three/fiber';
import { useMemo, useRef } from 'react';
import type { Group, Mesh } from 'three';

const ORCH: [number, number, number] = [-2.4, 0.7, 0];
const WORKER: [number, number, number] = [2.4, 0.7, 0];
const ESCROW: [number, number, number] = [0, -1.25, 0];
const SUB: [number, number, number] = [4.15, -0.85, 0];

function along(t: number): [number, number, number] {
  const clamped = ((t % 1) + 1) % 1;
  const [from, to, local] = clamped < 0.5 ? [ORCH, ESCROW, clamped / 0.5] : [ESCROW, WORKER, (clamped - 0.5) / 0.5];
  return [
    from[0] + (to[0] - from[0]) * local,
    from[1] + (to[1] - from[1]) * local,
    from[2] + (to[2] - from[2]) * local,
  ];
}

function Orchestrator() {
  const shell = useRef<Mesh>(null);
  useFrame(({ clock }) => {
    if (shell.current) shell.current.rotation.y = clock.elapsedTime * 0.6;
    if (shell.current) shell.current.rotation.x = clock.elapsedTime * 0.2;
  });
  return (
    <group position={ORCH}>
      <mesh>
        <sphereGeometry args={[0.48, 32, 32]} />
        <meshStandardMaterial color="#10b981" emissive="#10b981" emissiveIntensity={0.7} />
      </mesh>
      <mesh ref={shell}>
        <icosahedronGeometry args={[0.78, 1]} />
        <meshBasicMaterial color="#10b981" wireframe transparent opacity={0.85} />
      </mesh>
    </group>
  );
}

function WorkerNode() {
  const rings = useRef<Group>(null);
  useFrame(({ clock }) => {
    if (!rings.current) return;
    const scale = 1 + Math.sin(clock.elapsedTime * 2.2) * 0.06;
    rings.current.scale.setScalar(scale);
    rings.current.rotation.z = clock.elapsedTime * 0.35;
  });
  return (
    <group position={WORKER}>
      <mesh>
        <sphereGeometry args={[0.48, 32, 32]} />
        <meshStandardMaterial color="#06b6d4" emissive="#06b6d4" emissiveIntensity={0.7} />
      </mesh>
      <group ref={rings}>
        <mesh rotation={[Math.PI / 2, 0, 0]}>
          <torusGeometry args={[0.78, 0.012, 12, 64]} />
          <meshBasicMaterial color="#06b6d4" />
        </mesh>
        <mesh rotation={[Math.PI / 2.6, 0.4, 0]}>
          <torusGeometry args={[0.98, 0.008, 12, 64]} />
          <meshBasicMaterial color="#67e8f9" transparent opacity={0.7} />
        </mesh>
      </group>
    </group>
  );
}

function EscrowNode({ alert }: { alert: boolean }) {
  const ref = useRef<Mesh>(null);
  useFrame(({ clock }) => {
    if (!ref.current) return;
    ref.current.rotation.y = clock.elapsedTime * 0.45;
    ref.current.position.y = Math.sin(clock.elapsedTime * 1.3) * 0.06;
    const material = ref.current.material;
    if (!Array.isArray(material) && 'emissiveIntensity' in material) {
      material.emissiveIntensity = alert ? 0.4 + Math.abs(Math.sin(clock.elapsedTime * 8)) * 1.4 : 0.45;
    }
  });
  return (
    <group position={ESCROW}>
      <mesh ref={ref}>
        <icosahedronGeometry args={[0.58, 0]} />
        <meshStandardMaterial color={alert ? '#ef4444' : '#f59e0b'} emissive={alert ? '#ef4444' : '#f59e0b'} emissiveIntensity={0.45} metalness={0.75} roughness={0.25} />
      </mesh>
      {alert ? (
        <mesh>
          <octahedronGeometry args={[0.92, 0]} />
          <meshBasicMaterial color="#ef4444" wireframe transparent opacity={0.85} />
        </mesh>
      ) : null}
    </group>
  );
}

function EvaluatorRing() {
  const ring = useRef<Mesh>(null);
  useFrame(({ clock }) => {
    if (ring.current) ring.current.rotation.z = clock.elapsedTime * 0.8;
  });
  return (
    <group position={[ESCROW[0], ESCROW[1] + 1.05, ESCROW[2]]}>
      <mesh ref={ring}>
        <torusGeometry args={[0.34, 0.02, 12, 48]} />
        <meshBasicMaterial color="#f87171" />
      </mesh>
      <mesh>
        <sphereGeometry args={[0.08, 16, 16]} />
        <meshBasicMaterial color="#fecaca" />
      </mesh>
    </group>
  );
}

function alongPayout(t: number): [number, number, number] {
  const local = ((t % 1) + 1) % 1;
  return [
    ESCROW[0] + (WORKER[0] - ESCROW[0]) * local,
    ESCROW[1] + (WORKER[1] - ESCROW[1]) * local,
    ESCROW[2] + (WORKER[2] - ESCROW[2]) * local,
  ];
}

function alongRefund(t: number): [number, number, number] {
  const local = ((t % 1) + 1) % 1;
  return [
    ESCROW[0] + (ORCH[0] - ESCROW[0]) * local,
    ESCROW[1] + (ORCH[1] - ESCROW[1]) * local,
    ESCROW[2] + (ORCH[2] - ESCROW[2]) * local,
  ];
}

function alongHop(t: number): [number, number, number] {
  const local = ((t % 1) + 1) % 1;
  const legs: Array<[[number, number, number], [number, number, number]]> = [
    [ORCH, ESCROW],
    [ESCROW, WORKER],
    [WORKER, ESCROW],
    [ESCROW, SUB],
  ];
  const index = Math.min(legs.length - 1, Math.floor(local * legs.length));
  const span = (local * legs.length) - index;
  const [from, to] = legs[index];
  return [
    from[0] + (to[0] - from[0]) * span,
    from[1] + (to[1] - from[1]) * span,
    from[2] + (to[2] - from[2]) * span,
  ];
}

function SubWorker() {
  const ref = useRef<Mesh>(null);
  useFrame(({ clock }) => {
    if (ref.current) ref.current.rotation.y = clock.elapsedTime * 0.8;
  });
  return (
    <group position={SUB}>
      <mesh ref={ref}>
        <octahedronGeometry args={[0.36, 0]} />
        <meshStandardMaterial color="#a78bfa" emissive="#a78bfa" emissiveIntensity={0.7} />
      </mesh>
    </group>
  );
}

function ParticleStream({ active, reverse = false, payout = false, hop = false, slash = false }: { active: boolean; reverse?: boolean; payout?: boolean; hop?: boolean; slash?: boolean }) {
  const meshes = useRef<Array<Mesh | null>>([]);
  const started = useMemo(() => performance.now(), [active]);
  useFrame(() => {
    meshes.current.forEach((mesh, index) => {
      if (!mesh) return;
      const t = ((performance.now() - started) / 2200 + index * 0.12) % 1;
      const [x, y, z] = hop ? alongHop(t) : payout ? alongPayout(t) : reverse ? alongRefund(t) : along(t);
      mesh.position.set(x, y, z);
    });
  });
  if (!active) return null;
  return (
    <>
      {Array.from({ length: 8 }, (_, index) => (
        <mesh
          key={index}
          ref={(node) => {
            meshes.current[index] = node;
          }}
        >
          <sphereGeometry args={[0.045, 12, 12]} />
          <meshBasicMaterial color={slash ? '#ef4444' : hop ? (index % 2 === 0 ? '#67e8f9' : '#a78bfa') : payout ? (index % 2 === 0 ? '#10b981' : '#67e8f9') : reverse ? '#f59e0b' : '#fef3c7'} />
        </mesh>
      ))}
    </>
  );
}

export function MeshCanvas({ pulse, refundPulse = 0, meteredPulse = 0, hopPulse = 0, rejectPulse = 0 }: { pulse: number; refundPulse?: number; meteredPulse?: number; hopPulse?: number; rejectPulse?: number }) {
  return (
    <Canvas camera={{ position: [0, 1.15, 6.4], fov: 45 }} className="h-full w-full">
      <color attach="background" args={['#05070a']} />
      <ambientLight intensity={0.35} />
      <pointLight position={[-4, 2, 3]} intensity={28} color="#10b981" />
      <pointLight position={[4, 2, 3]} intensity={28} color="#06b6d4" />
      <pointLight position={[0, -1, 2]} intensity={18} color="#f59e0b" />
      <Orchestrator />
      <WorkerNode />
      <EscrowNode alert={rejectPulse > 0} />
      <EvaluatorRing />
      <SubWorker />
      <Line points={[ORCH, WORKER]} color="#10b981" dashed dashSize={0.16} gapSize={0.1} />
      <Line points={[WORKER, SUB]} color="#a78bfa" />
      <Line points={[ORCH, ESCROW]} color="#f59e0b" />
      <Line points={[WORKER, ESCROW]} color="#06b6d4" />
      <ParticleStream active={pulse > 0} />
      <ParticleStream active={refundPulse > 0 || meteredPulse > 0} reverse slash={rejectPulse > 0} />
      <ParticleStream active={meteredPulse > 0} payout />
      <ParticleStream active={hopPulse > 0} hop />
      <Grid
        position={[0, -1.9, 0]}
        args={[14, 14]}
        cellSize={0.45}
        cellThickness={0.6}
        sectionSize={2.2}
        sectionThickness={1}
        cellColor="#1e293b"
        sectionColor="#334155"
        fadeDistance={12}
        infiniteGrid
      />
      <OrbitControls
        enableDamping
        makeDefault
        minDistance={3.5}
        maxDistance={9}
        minPolarAngle={Math.PI / 6}
        maxPolarAngle={Math.PI / 1.7}
      />
    </Canvas>
  );
}
