'use client';

import { Line, OrbitControls } from '@react-three/drei';
import { Canvas, useFrame } from '@react-three/fiber';
import { useMemo, useRef } from 'react';
import type { Mesh } from 'three';

function Node({
  position,
  color,
  label,
}: {
  position: [number, number, number];
  color: string;
  label: 'sphere' | 'ico';
}) {
  const ref = useRef<Mesh>(null);
  useFrame(({ clock }) => {
    if (!ref.current) return;
    ref.current.rotation.y = clock.elapsedTime * 0.4;
    ref.current.position.y = position[1] + Math.sin(clock.elapsedTime * 1.4 + position[0]) * 0.08;
  });
  return (
    <group position={position}>
      <mesh ref={ref}>
        {label === 'sphere' ? <sphereGeometry args={[0.55, 32, 32]} /> : <icosahedronGeometry args={[0.62, 0]} />}
        <meshStandardMaterial color={color} emissive={color} emissiveIntensity={0.55} />
      </mesh>
      <mesh>
        <torusGeometry args={[0.9, 0.015, 12, 48]} />
        <meshBasicMaterial color={color} transparent opacity={0.7} />
      </mesh>
    </group>
  );
}

function Particles({ pulse }: { pulse: number }) {
  const ref = useRef<Mesh>(null);
  const start = useMemo(() => performance.now(), [pulse]);
  useFrame(() => {
    if (!ref.current) return;
    const t = ((performance.now() - start) / 1800) % 1;
    const path =
      t < 0.5
        ? [-2.2 + t * 2 * 2.2, 0.4 - t * 2 * 1.6, 0]
        : [0, -1.2 + (t - 0.5) * 2 * 1.6, 0];
    ref.current.position.set(path[0], path[1], path[2]);
  });
  if (!pulse) return null;
  return (
    <mesh ref={ref}>
      <sphereGeometry args={[0.08, 16, 16]} />
      <meshBasicMaterial color="#fff4c2" />
    </mesh>
  );
}

export function MeshCanvas({ pulse }: { pulse: number }) {
  return (
    <Canvas camera={{ position: [0, 1.2, 6.2], fov: 45 }} className="h-full w-full">
      <color attach="background" args={['#07090d']} />
      <ambientLight intensity={0.4} />
      <pointLight position={[4, 4, 4]} intensity={30} color="#3ee0c5" />
      <pointLight position={[-4, 2, 2]} intensity={20} color="#7aa2ff" />
      <Node position={[-2.2, 0.4, 0]} color="#34d399" label="sphere" />
      <Node position={[2.2, 0.4, 0]} color="#22d3ee" label="sphere" />
      <Node position={[0, -1.2, 0]} color="#e7b549" label="ico" />
      <Line points={[[-2.2, 0.4, 0], [2.2, 0.4, 0]]} color="#3ee0c5" dashed dashSize={0.18} gapSize={0.12} lineWidth={1} />
      <Line points={[[-2.2, 0.4, 0], [0, -1.2, 0]]} color="#e7b549" lineWidth={1} />
      <Line points={[[2.2, 0.4, 0], [0, -1.2, 0]]} color="#22d3ee" lineWidth={1} />
      <Particles pulse={pulse} />
      <OrbitControls enableDamping makeDefault minDistance={3.5} maxDistance={10} />
    </Canvas>
  );
}
