'use client';

import { Grid, Line, OrbitControls } from '@react-three/drei';
import { Canvas, useFrame } from '@react-three/fiber';
import { useMemo, useRef } from 'react';
import type { Group, Mesh } from 'three';

const ORCH: [number, number, number] = [-2.4, 0.7, 0];
const WORKER: [number, number, number] = [2.4, 0.7, 0];
const ESCROW: [number, number, number] = [0, -1.25, 0];

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

function EscrowNode() {
  const ref = useRef<Mesh>(null);
  useFrame(({ clock }) => {
    if (!ref.current) return;
    ref.current.rotation.y = clock.elapsedTime * 0.45;
    ref.current.position.y = Math.sin(clock.elapsedTime * 1.3) * 0.06;
  });
  return (
    <group position={ESCROW}>
      <mesh ref={ref}>
        <icosahedronGeometry args={[0.58, 0]} />
        <meshStandardMaterial color="#f59e0b" emissive="#f59e0b" emissiveIntensity={0.45} metalness={0.75} roughness={0.25} />
      </mesh>
    </group>
  );
}

function ParticleStream({ active }: { active: boolean }) {
  const meshes = useRef<Array<Mesh | null>>([]);
  const started = useMemo(() => performance.now(), [active]);
  useFrame(() => {
    meshes.current.forEach((mesh, index) => {
      if (!mesh) return;
      const t = ((performance.now() - started) / 2200 + index * 0.12) % 1;
      const [x, y, z] = along(t);
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
          <meshBasicMaterial color="#fef3c7" />
        </mesh>
      ))}
    </>
  );
}

export function MeshCanvas({ pulse }: { pulse: number }) {
  return (
    <Canvas camera={{ position: [0, 1.15, 6.4], fov: 45 }} className="h-full w-full">
      <color attach="background" args={['#05070a']} />
      <ambientLight intensity={0.35} />
      <pointLight position={[-4, 2, 3]} intensity={28} color="#10b981" />
      <pointLight position={[4, 2, 3]} intensity={28} color="#06b6d4" />
      <pointLight position={[0, -1, 2]} intensity={18} color="#f59e0b" />
      <Orchestrator />
      <WorkerNode />
      <EscrowNode />
      <Line points={[ORCH, WORKER]} color="#10b981" dashed dashSize={0.16} gapSize={0.1} />
      <Line points={[ORCH, ESCROW]} color="#f59e0b" />
      <Line points={[WORKER, ESCROW]} color="#06b6d4" />
      <ParticleStream active={pulse > 0} />
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
